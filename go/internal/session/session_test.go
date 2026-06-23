// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionRoundTrip(t *testing.T) {
	// Isolate the data directory so the test never touches real sessions.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	s := New("examples.taskflows.echo", map[string]string{"fruit": "apple"}, "hi", 3, "examples.model_configs.model_config")
	require.Len(t, s.SessionID, 12)
	require.Equal(t, 0, s.NextTaskIndex())

	require.NoError(t, s.RecordTask(0, "first", true, []string{"r0"}))
	require.Equal(t, 1, s.NextTaskIndex())

	loaded, err := Load(s.SessionID)
	require.NoError(t, err)
	require.Equal(t, s.SessionID, loaded.SessionID)
	require.Equal(t, "examples.taskflows.echo", loaded.TaskflowPath)
	require.Equal(t, []string{"r0"}, loaded.LastToolResults)
	require.Equal(t, 1, loaded.NextTaskIndex())
	require.False(t, loaded.Finished)

	require.NoError(t, loaded.MarkFinished())
	again, err := Load(s.SessionID)
	require.NoError(t, err)
	require.True(t, again.Finished)
}

func TestLoadMissing(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_, err := Load("doesnotexist")
	require.Error(t, err)
}
