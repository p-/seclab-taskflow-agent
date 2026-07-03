// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package anthropic

import (
	"context"
	"encoding/json"
	"io"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// validReasoning is the set of reasoning efforts accepted by the backend,
// matching the Python anthropic_sdk backend.
var validReasoning = map[string]bool{"low": true, "medium": true, "high": true, "max": true}

// stream implements sdk.Stream over a buffered channel fed by the agent loop
// running in a background goroutine. It mirrors the openai backend's stream.
type stream struct {
	events chan sdk.StreamEvent
	errc   chan error
	cancel context.CancelFunc
	err    error
	done   bool
}

// RunStreamed starts the agent loop and returns a Stream of neutral events.
func (b *Backend) RunStreamed(ctx context.Context, a sdk.Agent, prompt string, maxTurns int) (sdk.Stream, error) {
	ag, ok := a.(*agent)
	if !ok {
		return nil, &sdk.UnexpectedError{Msg: "anthropic backend: invalid agent handle"}
	}
	runCtx, cancel := context.WithCancel(ctx)
	s := &stream{
		events: make(chan sdk.StreamEvent, 16),
		errc:   make(chan error, 1),
		cancel: cancel,
	}
	go ag.run(runCtx, prompt, maxTurns, s)
	return s, nil
}

// Recv returns the next event, or io.EOF when the run completes normally.
func (s *stream) Recv() (sdk.StreamEvent, error) {
	if s.done {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	ev, ok := <-s.events
	if ok {
		return ev, nil
	}
	// Channel closed: drain the terminal error.
	s.done = true
	select {
	case err := <-s.errc:
		s.err = err
	default:
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

// Close cancels the run and releases the loop goroutine.
func (s *stream) Close() error {
	s.cancel()
	return nil
}

// run executes the Messages API agent loop. It maintains a growing message
// list (assistant tool_use turns + user tool_result turns) across turns,
// emitting TextDelta and ToolEnd events, closing s.events on completion and
// leaving a terminal error on s.errc when one occurs. It ports the Python
// AnthropicSDKBackend.run_streamed.
func (a *agent) run(ctx context.Context, prompt string, maxTurns int, s *stream) {
	defer close(s.events)

	base, err := a.baseParams()
	if err != nil {
		s.errc <- err
		return
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
	}

	for turn := 0; turn < maxTurns; turn++ {
		params := base
		params.Messages = messages

		respStream := a.client.Messages.NewStreaming(ctx, params)
		final := anthropic.Message{}
		for respStream.Next() {
			ev := respStream.Current()
			if ierr := final.Accumulate(ev); ierr != nil {
				s.errc <- &sdk.UnexpectedError{Msg: ierr.Error()}
				_ = respStream.Close()
				return
			}
			if ev.Type == "content_block_delta" {
				if d := ev.Delta.Text; d != "" {
					if !a.emit(ctx, s, sdk.TextDelta{Text: d}) {
						_ = respStream.Close()
						return
					}
				} else if d := ev.Delta.Thinking; d != "" && a.streamThinking {
					if !a.emit(ctx, s, sdk.TextDelta{Text: d}) {
						_ = respStream.Close()
						return
					}
				}
			}
		}
		if ierr := respStream.Err(); ierr != nil {
			s.errc <- mapError(ierr)
			return
		}

		if final.StopReason != anthropic.StopReasonToolUse {
			return // end_turn / max_tokens / stop_sequence -> io.EOF
		}

		toolUses := toolUseBlocks(final)
		if len(toolUses) == 0 {
			return
		}

		// Echo the assistant's tool_use turn into the conversation.
		messages = append(messages, final.ToParam())

		toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(toolUses))
		for _, block := range toolUses {
			result, ierr := a.dispatchTool(ctx, block.Name, block.Input)
			isErr := false
			if ierr != nil {
				result = "Error calling " + block.Name + ": " + ierr.Error()
				isErr = true
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, result, isErr))
			if !a.emit(ctx, s, sdk.ToolEnd{ToolName: block.Name, Text: result}) {
				return
			}
		}

		// exclude_from_context: stop after tool results are emitted so they
		// are available to the runner but not fed back into the model
		// context (matches the openai backend and Python behaviour).
		if a.exclude {
			return
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	s.errc <- &sdk.MaxTurnsError{Msg: "maximum turns exceeded"}
}

// emit sends an event, returning false (after recording a terminal error) if
// the run context is cancelled first.
func (a *agent) emit(ctx context.Context, s *stream, ev sdk.StreamEvent) bool {
	select {
	case s.events <- ev:
		return true
	case <-ctx.Done():
		s.errc <- mapError(ctx.Err())
		return false
	}
}

// baseParams builds the per-request parameters shared across turns from the
// resolved model_settings, porting the Python create_kwargs construction:
// temperature/top_p pass-through, adaptive thinking with an effort-based
// output_config, and automatic ephemeral prompt caching.
func (a *agent) baseParams() (anthropic.MessageNewParams, error) {
	systemBlock := anthropic.TextBlockParam{Text: a.system}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: a.maxTokens,
	}
	if len(a.tools) > 0 {
		params.Tools = a.tools
	}

	if v, ok := floatSetting(a.settings, "temperature"); ok {
		params.Temperature = param.NewOpt(v)
	}
	if v, ok := floatSetting(a.settings, "top_p"); ok {
		params.TopP = param.NewOpt(v)
	}

	if reasoning, ok := a.settings["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && effort != "" {
			if !validReasoning[effort] {
				return params, &sdk.BadRequestError{Msg: "anthropic_sdk: invalid reasoning effort " + effort + " (expected one of low, medium, high, max)"}
			}
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}
			params.OutputConfig = anthropic.OutputConfigParam{
				Effort: anthropic.OutputConfigEffort(effort),
			}
		}
	}

	// Automatic prompt caching: place an ephemeral cache breakpoint on the
	// system block, which caches the static tools+system prefix (Anthropic
	// caches in tools -> system -> messages order). cache_control is a
	// content-block field, not a top-level request param -- setting it at the
	// top level is rejected by the Messages API / CAPI proxy. Default on;
	// explicit opt-out (prompt_caching: false) for proxies that do not
	// support cache_control. A string value selects a non-default TTL. The
	// TTL is always set explicitly so the block is serialised (a zero-value
	// CacheControlEphemeralParam is omitted).
	enabled, ttl := promptCaching(a.settings)
	if enabled && a.system != "" {
		if ttl == "" {
			ttl = "5m"
		}
		systemBlock.CacheControl = anthropic.CacheControlEphemeralParam{
			TTL: anthropic.CacheControlEphemeralTTL(ttl),
		}
	}
	params.System = []anthropic.TextBlockParam{systemBlock}

	return params, nil
}

// promptCaching resolves the prompt_caching model setting. It defaults to
// enabled with a 5m TTL; a bool toggles it; a string enables it with a custom
// TTL.
func promptCaching(settings map[string]any) (bool, string) {
	v, ok := settings["prompt_caching"]
	if !ok {
		return true, ""
	}
	switch val := v.(type) {
	case bool:
		return val, ""
	case string:
		return true, val
	default:
		return true, ""
	}
}

func floatSetting(settings map[string]any, key string) (float64, bool) {
	v, ok := settings[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// toolUseBlocks extracts the tool_use content blocks from a final message.
func toolUseBlocks(msg anthropic.Message) []anthropic.ToolUseBlock {
	var blocks []anthropic.ToolUseBlock
	for _, c := range msg.Content {
		if c.Type == "tool_use" {
			blocks = append(blocks, c.AsToolUse())
		}
	}
	return blocks
}

// dispatchTool routes a namespaced tool call to the owning MCP server.
func (a *agent) dispatchTool(ctx context.Context, name string, rawArgs json.RawMessage) (string, error) {
	var args map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return "", err
		}
	}
	for _, srv := range a.servers {
		if srv.HandlesTool(name) {
			return srv.CallTool(ctx, name, args)
		}
	}
	return "", errNoServer(name)
}

type errNoServer string

func (e errNoServer) Error() string { return "no MCP server handles tool " + string(e) }
