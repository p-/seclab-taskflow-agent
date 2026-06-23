// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

//go:build e2e

// Package e2e contains end-to-end smoke tests that exercise the full agent
// against the real Python echo MCP server and a stubbed OpenAI streaming
// endpoint. Run with: go test -tags e2e ./internal/e2e/...
//
// Requirements: a Python interpreter on PATH (or via STFA_PYTHON) with the
// seclab_taskflow_agent package and fastmcp installed.
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

// stubOpenAI returns an httptest server that, on the first chat-completions
// request, asks the model to call the namespaced echo tool, and on the second
// returns a final text answer.
func stubOpenAI(t *testing.T, toolName string) *httptest.Server {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
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
			write("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"" + toolName + "\",\"arguments\":\"{\\\"message\\\":\\\"hi\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
			write("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			write("data: {\"id\":\"c2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"All done\"},\"finish_reason\":null}]}\n\n")
			write("data: {\"id\":\"c2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"stub\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		}
		write("data: [DONE]\n\n")
	})
	return httptest.NewServer(mux)
}

func repoRoot(t *testing.T) string {
	abs, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	return abs
}

func TestEchoTaskflowEndToEnd(t *testing.T) {
	python := os.Getenv("STFA_PYTHON")
	if python == "" {
		python = filepath.Join(repoRoot(t), ".venv", "bin", "python")
	}
	if _, err := os.Stat(python); err != nil {
		t.Skipf("python interpreter not found at %s", python)
	}

	// The echo toolbox launches ``python -m ...``; point it at the venv and
	// ensure the package is importable from the repo root.
	t.Setenv("PATH", filepath.Dir(python)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SECLAB_TASKFLOW_PATH", repoRoot(t))
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOG_DIR", t.TempDir())

	toolName := mcp.CompressName("seclab_taskflow_agent.toolboxes.echo") + "echo_tool"
	stub := stubOpenAI(t, toolName)
	defer stub.Close()
	t.Setenv("AI_API_ENDPOINT", stub.URL)
	t.Setenv("AI_API_TOKEN", "stub-token")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	at := loader.New()
	err := runner.RunMain(ctx, at, runner.Options{
		Taskflow: "examples.taskflows.echo",
	})
	require.NoError(t, err)
}
