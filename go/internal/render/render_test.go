// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package render

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestAsyncOutputBufferingAndFlush(t *testing.T) {
	t.Setenv("LOG_DIR", t.TempDir())
	mu.Lock()
	asyncBuffers = map[string]*strings.Builder{}
	mu.Unlock()

	out := captureStdout(t, func() {
		OutputMaybeBuffered("first", true, "task-1")
		OutputMaybeBuffered(" second", true, "task-1")
	})
	require.Equal(t, "** 🤖✏️ Gathering output from async task ... please hold\n", out)

	out = captureStdout(t, func() {
		FlushAsyncOutput("task-1")
	})
	require.Equal(t, "** 🤖✏️ Output for async task: task-1\n\nfirst second", out)

	out = captureStdout(t, func() {
		FlushAsyncOutput("task-1")
	})
	require.Empty(t, out)
}

func TestOutputMaybeBufferedWritesImmediatelyForNormalTasks(t *testing.T) {
	out := captureStdout(t, func() {
		OutputMaybeBuffered("plain", false, "")
	})
	require.Equal(t, "plain", out)
}
