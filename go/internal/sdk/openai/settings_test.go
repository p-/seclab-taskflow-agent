// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
)

func TestForwardSettingsOptionsEmpty(t *testing.T) {
	require.Nil(t, forwardSettingsOptions(nil))
	require.Nil(t, forwardSettingsOptions(map[string]any{}))
}

// settings supplied in model_settings must be forwarded to the Chat
// Completions request body verbatim, mirroring the Python implementation.
func TestChatForwardsModelSettings(t *testing.T) {
	var body string
	stub := newCaptureStub(t, "/chat/completions", &body,
		"data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n")
	defer stub.Close()

	ag := &agent{
		client: oai.NewClient(option.WithBaseURL(stub.URL), option.WithAPIKey("test")),
		model:  "stub",
		system: "system",
		settingsOpts: forwardSettingsOptions(map[string]any{
			"temperature": 0.5,
			"top_p":       0.9,
			"reasoning":   map[string]any{"effort": "high"},
		}),
	}

	collectOpenAIEvents(t, ag, "prompt")
	require.Contains(t, body, "\"temperature\":0.5")
	require.Contains(t, body, "\"top_p\":0.9")
	require.Contains(t, body, "\"reasoning\":{\"effort\":\"high\"}")
}

// The same forwarding must apply to the Responses API request body.
func TestResponsesForwardsModelSettings(t *testing.T) {
	var body string
	stub := newCaptureStub(t, "/responses", &body,
		"data: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":{\"output\":[{\"type\":\"message\",\"id\":\"m1\",\"role\":\"assistant\",\"content\":[]}]}}\n\n")
	defer stub.Close()

	ag := &agent{
		client:  oai.NewClient(option.WithBaseURL(stub.URL), option.WithAPIKey("test")),
		model:   "stub",
		system:  "system",
		apiType: grammar.APITypeResponses,
		settingsOpts: forwardSettingsOptions(map[string]any{
			"temperature": 1,
			"reasoning":   map[string]any{"effort": "high"},
		}),
	}

	collectOpenAIEvents(t, ag, "prompt")
	require.Contains(t, body, "\"temperature\":1")
	require.Contains(t, body, "\"reasoning\":{\"effort\":\"high\"}")
}

// newCaptureStub returns an SSE stub server that records the request body of
// the first call into *body and replies with the given event payload.
func newCaptureStub(t *testing.T, path string, body *string, payload string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, path, r.URL.Path)
		if *body == "" {
			b, _ := io.ReadAll(r.Body)
			*body = string(b)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		fmt.Fprint(w, payload)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flush != nil {
			flush.Flush()
		}
	}))
}
