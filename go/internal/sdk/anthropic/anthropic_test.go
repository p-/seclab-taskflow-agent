// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

func TestName(t *testing.T) {
	require.Equal(t, "anthropic_sdk", (&Backend{}).Name())
}

func TestValidateRejectsHandoffs(t *testing.T) {
	err := (&Backend{}).Validate(&sdk.AgentSpec{
		Model:    "claude",
		Handoffs: []*sdk.AgentSpec{{Name: "other"}},
	})
	require.Error(t, err)
	var capErr *sdk.CapabilityError
	require.ErrorAs(t, err, &capErr)
}

func TestValidateRequiresModel(t *testing.T) {
	err := (&Backend{}).Validate(&sdk.AgentSpec{})
	require.Error(t, err)
	var badErr *sdk.BadRequestError
	require.ErrorAs(t, err, &badErr)
}

func TestValidateAllowsMessages(t *testing.T) {
	err := (&Backend{}).Validate(&sdk.AgentSpec{
		Model:              "claude",
		APIType:            "messages",
		ExcludeFromContext: true,
	})
	require.NoError(t, err)
}

func TestToToolParamConvertsSchema(t *testing.T) {
	tp := toToolParam(mcp.Tool{
		Name:        "ns__do_thing",
		Description: "Does a thing",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"x": map[string]any{"type": "string"}},
			"required":   []any{"x"},
		},
	})
	require.NotNil(t, tp.OfTool)
	require.Equal(t, "ns__do_thing", tp.OfTool.Name)
	require.Equal(t, "Does a thing", tp.OfTool.Description.Value)
	require.Equal(t, []string{"x"}, tp.OfTool.InputSchema.Required)
	require.NotNil(t, tp.OfTool.InputSchema.Properties)
}

func TestResolveMaxTokens(t *testing.T) {
	require.Equal(t, int64(defaultMaxTokens), resolveMaxTokens(nil))
	require.Equal(t, int64(defaultMaxTokens), resolveMaxTokens(map[string]any{}))
	require.Equal(t, int64(2048), resolveMaxTokens(map[string]any{"max_tokens": 2048}))
	require.Equal(t, int64(4096), resolveMaxTokens(map[string]any{"max_tokens": float64(4096)}))
}

func TestBaseParamsReasoning(t *testing.T) {
	ag := &agent{model: "claude", maxTokens: 100, system: "sys", settings: map[string]any{
		"temperature": 0.5,
		"top_p":       0.9,
		"reasoning":   map[string]any{"effort": "high"},
	}}
	params, err := ag.baseParams()
	require.NoError(t, err)
	require.NotNil(t, params.Thinking.OfAdaptive)
	require.Equal(t, anthropic.OutputConfigEffort("high"), params.OutputConfig.Effort)
	require.Equal(t, 0.5, params.Temperature.Value)
	require.Equal(t, 0.9, params.TopP.Value)
	// Prompt caching defaults on: breakpoint sits on the system block.
	require.Len(t, params.System, 1)
	require.NotEqual(t, anthropic.CacheControlEphemeralParam{}, params.System[0].CacheControl)
}

func TestBaseParamsInvalidReasoning(t *testing.T) {
	ag := &agent{model: "claude", maxTokens: 100, settings: map[string]any{
		"reasoning": map[string]any{"effort": "bogus"},
	}}
	_, err := ag.baseParams()
	require.Error(t, err)
	var badErr *sdk.BadRequestError
	require.ErrorAs(t, err, &badErr)
}

func TestBaseParamsPromptCachingOptOut(t *testing.T) {
	ag := &agent{model: "claude", maxTokens: 100, system: "sys", settings: map[string]any{
		"prompt_caching": false,
	}}
	params, err := ag.baseParams()
	require.NoError(t, err)
	require.Len(t, params.System, 1)
	require.Equal(t, anthropic.CacheControlEphemeralParam{}, params.System[0].CacheControl)
}

// TestStreamTextCompletion drives a text-only Messages response and asserts
// the backend emits the text deltas and terminates cleanly.
func TestStreamTextCompletion(t *testing.T) {
	body := sseFrames(
		`message_start`, `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"claude","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`content_block_start`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`content_block_delta`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`,
		`content_block_delta`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}`,
		`content_block_stop`, `{"type":"content_block_stop","index":0}`,
		`message_delta`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`message_stop`, `{"type":"message_stop"}`,
	)
	stub := newSSEStub(t, body, nil)
	defer stub.Close()

	ag := newTestAgent(stub.URL)
	events := collectEvents(t, ag, "hi")
	require.Len(t, events, 2)
	require.Equal(t, sdk.TextDelta{Text: "Hello "}, events[0])
	require.Equal(t, sdk.TextDelta{Text: "world"}, events[1])
}

// TestExcludeFromContextStopsAfterToolResults mirrors the openai backend test:
// on a tool_use turn with exclude=true the loop emits a single ToolEnd and
// stops without a second API call.
func TestExcludeFromContextStopsAfterToolResults(t *testing.T) {
	var calls int32
	toolFrames := sseFrames(
		`message_start`, `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"claude","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`content_block_start`, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu_1","name":"fake_tool","input":{}}}`,
		`content_block_delta`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`content_block_stop`, `{"type":"content_block_stop","index":0}`,
		`message_delta`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`,
		`message_stop`, `{"type":"message_stop"}`,
	)
	stub := newSSEStub(t, toolFrames, &calls)
	defer stub.Close()

	ag := newTestAgent(stub.URL)
	ag.exclude = true
	ag.tools = []anthropic.ToolUnionParam{toToolParam(fakeMCPTool())}

	events := collectEvents(t, ag, "call it")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	require.Len(t, events, 1)
	tool, ok := events[0].(sdk.ToolEnd)
	require.True(t, ok)
	require.Equal(t, "fake_tool", tool.ToolName)
	// No MCP server is wired, so dispatch reports the tool as unhandled.
	require.Contains(t, tool.Text, "Error calling fake_tool")
}

func fakeMCPTool() mcp.Tool {
	return mcp.Tool{
		Name:        "fake_tool",
		Description: "Fake tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func newTestAgent(baseURL string) *agent {
	return &agent{
		client:    anthropic.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey("test")),
		model:     "claude",
		system:    "system",
		maxTokens: 128,
	}
}

// sseFrames builds an event-stream body from alternating (eventType, jsonData)
// pairs.
func sseFrames(pairs ...string) string {
	out := ""
	for i := 0; i+1 < len(pairs); i += 2 {
		out += fmt.Sprintf("event: %s\ndata: %s\n\n", pairs[i], pairs[i+1])
	}
	return out
}

// newSSEStub serves the given event-stream body, optionally counting requests.
func newSSEStub(t *testing.T, body string, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

func collectEvents(t *testing.T, ag *agent, prompt string) []sdk.StreamEvent {
	t.Helper()
	st, err := (&Backend{}).RunStreamed(context.Background(), ag, prompt, 5)
	require.NoError(t, err)
	defer st.Close()

	var events []sdk.StreamEvent
	for {
		ev, err := st.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, ev)
	}
	return events
}
