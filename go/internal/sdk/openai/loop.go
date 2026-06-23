// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	oai "github.com/openai/openai-go"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// stream implements sdk.Stream over a buffered channel fed by the agent loop
// running in a background goroutine.
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
		return nil, &sdk.UnexpectedError{Msg: "openai backend: invalid agent handle"}
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

// run dispatches to the API-specific agent loop based on the agent's apiType.
func (a *agent) run(ctx context.Context, prompt string, maxTurns int, s *stream) {
	if a.apiType == "responses" {
		a.runResponses(ctx, prompt, maxTurns, s)
		return
	}
	a.runChat(ctx, prompt, maxTurns, s)
}

// Recv returns the next event, or io.EOF when the run completes normally.
func (s *stream) Recv() (sdk.StreamEvent, error) {
	if s.done {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	select {
	case ev, ok := <-s.events:
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
}

// Close cancels the run and releases the loop goroutine.
func (s *stream) Close() error {
	s.cancel()
	return nil
}

// runChat executes the Chat Completions agent loop, emitting TextDelta and
// ToolEnd events. It terminates by closing s.events, optionally leaving a
// terminal error on s.errc.
func (a *agent) runChat(ctx context.Context, prompt string, maxTurns int, s *stream) {
	defer close(s.events)

	messages := []oai.ChatCompletionMessageParamUnion{
		oai.SystemMessage(a.system),
		oai.UserMessage(prompt),
	}

	for turn := 0; turn < maxTurns; turn++ {
		params := oai.ChatCompletionNewParams{
			Model:    a.model,
			Messages: messages,
		}
		if len(a.tools) > 0 {
			params.Tools = a.tools
		}
		if a.temp != nil {
			params.Temperature = oai.Float(*a.temp)
		}

		chatStream := a.client.Chat.Completions.NewStreaming(ctx, params)
		acc := oai.ChatCompletionAccumulator{}
		for chatStream.Next() {
			chunk := chatStream.Current()
			acc.AddChunk(chunk)
			if len(chunk.Choices) > 0 {
				if d := chunk.Choices[0].Delta.Content; d != "" {
					select {
					case s.events <- sdk.TextDelta{Text: d}:
					case <-ctx.Done():
						s.errc <- mapError(ctx.Err())
						_ = chatStream.Close()
						return
					}
				}
			}
		}
		if err := chatStream.Err(); err != nil {
			s.errc <- mapError(err)
			return
		}

		if len(acc.Choices) == 0 {
			return // nothing more to do
		}
		msg := acc.Choices[0].Message
		messages = append(messages, msg.ToParam())

		if len(msg.ToolCalls) == 0 {
			return // natural completion -> io.EOF
		}

		for _, tc := range msg.ToolCalls {
			result, err := a.dispatchTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = "Tool call failed: " + err.Error()
			}
			select {
			case s.events <- sdk.ToolEnd{ToolName: tc.Function.Name, Text: result}:
			case <-ctx.Done():
				s.errc <- mapError(ctx.Err())
				return
			}
			messages = append(messages, oai.ToolMessage(result, tc.ID))
		}
	}

	s.errc <- &sdk.MaxTurnsError{Msg: "maximum turns exceeded"}
}

// dispatchTool routes a namespaced tool call to the owning MCP server.
func (a *agent) dispatchTool(ctx context.Context, name, rawArgs string) (string, error) {
	var args map[string]any
	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return "", err
		}
	}
	for _, srv := range a.servers {
		if srv.HandlesTool(name) {
			return srv.CallTool(ctx, name, args)
		}
	}
	return "", errors.New("no MCP server handles tool " + name)
}
