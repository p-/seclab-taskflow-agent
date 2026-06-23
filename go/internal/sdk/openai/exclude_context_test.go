// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

func TestValidateAllowsExcludeFromContext(t *testing.T) {
	err := (&Backend{}).Validate(&sdk.AgentSpec{
		APIType:            grammar.APITypeChatCompletions,
		ExcludeFromContext: true,
	})
	require.NoError(t, err)
}

func TestChatExcludeFromContextStopsAfterToolResults(t *testing.T) {
	var calls int32
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		write := func(s string) {
			fmt.Fprint(w, s)
			if flush != nil {
				flush.Flush()
			}
		}
		if n == 1 {
			write("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"fake_tool\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n")
			write("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			write("data: {\"id\":\"c2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"unexpected\"},\"finish_reason\":null}]}\n\n")
		}
		write("data: [DONE]\n\n")
	}))
	defer stub.Close()

	ag := &agent{
		client:  oai.NewClient(option.WithBaseURL(stub.URL), option.WithAPIKey("test")),
		model:   "stub",
		system:  "system",
		tools:   []oai.ChatCompletionToolParam{toToolParam(fakeMCPTool())},
		exclude: true,
	}

	events := collectOpenAIEvents(t, ag, "prompt")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	require.Len(t, events, 1)
	tool, ok := events[0].(sdk.ToolEnd)
	require.True(t, ok)
	require.Equal(t, "fake_tool", tool.ToolName)
	require.Contains(t, tool.Text, "Tool call failed")
}

func TestResponsesExcludeFromContextStopsAfterToolResults(t *testing.T) {
	var calls int32
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/responses", r.URL.Path)
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		write := func(s string) {
			fmt.Fprint(w, s)
			if flush != nil {
				flush.Flush()
			}
		}
		if n == 1 {
			write("data: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":{\"output\":[{\"type\":\"function_call\",\"id\":\"fc1\",\"call_id\":\"call_1\",\"name\":\"fake_tool\",\"arguments\":\"{}\"}]}}\n\n")
		} else {
			write("data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"m1\",\"output_index\":0,\"content_index\":0,\"delta\":\"unexpected\"}\n\n")
			write("data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"output\":[{\"type\":\"message\",\"id\":\"m1\",\"role\":\"assistant\",\"content\":[]}]}}\n\n")
		}
		write("data: [DONE]\n\n")
	}))
	defer stub.Close()

	ag := &agent{
		client:    oai.NewClient(option.WithBaseURL(stub.URL), option.WithAPIKey("test")),
		model:     "stub",
		system:    "system",
		apiType:   grammar.APITypeResponses,
		respTools: []responses.ToolUnionParam{toResponsesToolParam(fakeMCPTool())},
		exclude:   true,
	}

	events := collectOpenAIEvents(t, ag, "prompt")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	require.Len(t, events, 1)
	tool, ok := events[0].(sdk.ToolEnd)
	require.True(t, ok)
	require.Equal(t, "fake_tool", tool.ToolName)
	require.Contains(t, tool.Text, "Tool call failed")
}

func fakeMCPTool() mcp.Tool {
	return mcp.Tool{
		Name:        "fake_tool",
		Description: "Fake tool",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func collectOpenAIEvents(t *testing.T, ag *agent, prompt string) []sdk.StreamEvent {
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
