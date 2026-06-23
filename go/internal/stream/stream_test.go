// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package stream

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/render"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

type fakeAgent struct{}

func (fakeAgent) Close(context.Context) error { return nil }

type fakeBackend struct {
	events []sdk.StreamEvent
}

func (f fakeBackend) Name() string                  { return "fake" }
func (f fakeBackend) Validate(*sdk.AgentSpec) error { return nil }
func (f fakeBackend) Build(context.Context, *sdk.AgentSpec) (sdk.Agent, error) {
	return fakeAgent{}, nil
}
func (f fakeBackend) RunStreamed(context.Context, sdk.Agent, string, int) (sdk.Stream, error) {
	return &fakeStream{events: f.events}, nil
}

type fakeStream struct {
	events []sdk.StreamEvent
	idx    int
}

func (f *fakeStream) Recv() (sdk.StreamEvent, error) {
	if f.idx >= len(f.events) {
		return nil, io.EOF
	}
	ev := f.events[f.idx]
	f.idx++
	return ev, nil
}

func (*fakeStream) Close() error { return nil }

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	require.NoError(t, w.Close())
	os.Stdout = old
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return buf.String()
}

func TestDriveBuffersAsyncTextAndForwardsToolHooks(t *testing.T) {
	t.Setenv("LOG_DIR", t.TempDir())
	backend := fakeBackend{events: []sdk.StreamEvent{
		sdk.TextDelta{Text: "hello"},
		sdk.ToolEnd{ToolName: "tool", Text: "result"},
	}}
	var started []string
	var ended []string

	out := captureStdout(t, func() {
		err := Drive(
			context.Background(),
			backend,
			fakeAgent{},
			"prompt",
			1,
			func(name string, asyncTask bool, taskID string) {
				started = append(started, name)
				render.OutputMaybeBuffered("tool-start:"+name, asyncTask, taskID)
			},
			func(name, result string) {
				ended = append(ended, name+":"+result)
			},
			true,
			"stream-task",
		)
		require.NoError(t, err)
		render.FlushAsyncOutput("stream-task")
	})

	require.Equal(t, []string{"tool"}, started)
	require.Equal(t, []string{"tool:result"}, ended)
	require.Contains(t, out, "Gathering output from async task")
	require.Contains(t, out, "Output for async task: stream-task")
	require.Contains(t, out, "hello")
	require.Contains(t, out, "tool-start:tool")
}
