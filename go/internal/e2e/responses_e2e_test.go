// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/runner"
)

// stubResponses returns an httptest server speaking the Responses API SSE
// protocol: the first request emits a function call to the echo tool, the
// second streams a final text answer.
func stubResponses(t *testing.T, toolName string) *httptest.Server {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/responses", func(w http.ResponseWriter, r *http.Request) {
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
			write("data: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":{\"output\":[{\"type\":\"function_call\",\"id\":\"fc1\",\"call_id\":\"call_1\",\"name\":\"" + toolName + "\",\"arguments\":\"{\\\"message\\\":\\\"hi\\\"}\"}]}}\n\n")
		} else {
			write("data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"m1\",\"output_index\":0,\"content_index\":0,\"delta\":\"All done\"}\n\n")
			write("data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"output\":[{\"type\":\"message\",\"id\":\"m1\",\"role\":\"assistant\",\"content\":[]}]}}\n\n")
		}
		write("data: [DONE]\n\n")
	})
	return httptest.NewServer(mux)
}

// writeFile writes content to path, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestResponsesAPIEndToEnd(t *testing.T) {
	python := os.Getenv("STFA_PYTHON")
	if python == "" {
		python = filepath.Join(repoRoot(t), ".venv", "bin", "python")
	}
	if _, err := os.Stat(python); err != nil {
		t.Skipf("python interpreter not found at %s", python)
	}

	// Temp fixtures: a taskflow + a model config selecting api_type responses
	// without endpoint/token overrides (so it uses the stub via AI_API_*).
	fixtureRoot := t.TempDir()
	writeFile(t, filepath.Join(fixtureRoot, "fixtures", "mc.yaml"), `
seclab-taskflow-agent:
  version: "1.0"
  filetype: model_config
models:
  gpt_responses: gpt-4.1
model_settings:
  gpt_responses:
    api_type: responses
`)
	writeFile(t, filepath.Join(fixtureRoot, "fixtures", "flow.yaml"), `
seclab-taskflow-agent:
  version: "1.0"
  filetype: taskflow
model_config: fixtures.mc
taskflow:
  - task:
      max_steps: 5
      must_complete: true
      model: gpt_responses
      agents:
        - examples.personalities.echo
      user_prompt: |
        Hello from the Responses API
`)

	t.Setenv("PATH", filepath.Dir(python)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SECLAB_TASKFLOW_PATH", fixtureRoot+string(os.PathListSeparator)+repoRoot(t))
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOG_DIR", t.TempDir())

	toolName := mcp.CompressName("seclab_taskflow_agent.toolboxes.echo") + "echo_tool"
	stub := stubResponses(t, toolName)
	defer stub.Close()
	t.Setenv("AI_API_ENDPOINT", stub.URL)
	t.Setenv("AI_API_TOKEN", "stub-token")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	at := loader.New()
	err := runner.RunMain(ctx, at, runner.Options{Taskflow: "fixtures.flow"})
	require.NoError(t, err)
}
