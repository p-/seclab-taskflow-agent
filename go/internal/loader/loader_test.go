// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package loader

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// repoRoot returns the repository root (two levels above this package's go/
// module directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	// test runs in go/internal/loader; repo root is ../../..
	abs, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	return abs
}

func TestDottedToRelPath(t *testing.T) {
	require.Equal(t, filepath.Join("examples", "taskflows", "echo.yaml"),
		dottedToRelPath("examples.taskflows.echo"))
}

func TestLoadTaskflowFromRepo(t *testing.T) {
	t.Setenv(SearchPathEnv, repoRoot(t))
	a := New()
	doc, err := a.GetTaskflow("examples.taskflows.echo")
	require.NoError(t, err)
	require.Equal(t, "1.0", doc.Header.Version)
	require.Equal(t, "taskflow", doc.Header.Filetype)
	require.NotEmpty(t, doc.Taskflow)
}

func TestLoadEveryExampleTaskflow(t *testing.T) {
	t.Setenv(SearchPathEnv, repoRoot(t))
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "examples", "taskflows", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	a := New()
	for _, m := range matches {
		stem := filepath.Base(m)
		stem = stem[:len(stem)-len(".yaml")]
		dotted := "examples.taskflows." + stem
		_, err := a.GetTaskflow(dotted)
		require.NoError(t, err, "loading %s", dotted)
	}
}

func TestEmbeddedToolboxFallback(t *testing.T) {
	// No search path set; must resolve from the embedded snapshot.
	t.Setenv(SearchPathEnv, "/nonexistent")
	a := New()
	doc, err := a.GetToolbox("seclab_taskflow_agent.toolboxes.echo")
	require.NoError(t, err)
	require.Equal(t, "stdio", doc.ServerParams.Kind)
}

func TestFiletypeMismatch(t *testing.T) {
	t.Setenv(SearchPathEnv, repoRoot(t))
	a := New()
	// echo is a taskflow, not a personality.
	_, err := a.GetPersonality("examples.taskflows.echo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected filetype")
}

func TestCachingReturnsSamePointer(t *testing.T) {
	t.Setenv(SearchPathEnv, repoRoot(t))
	a := New()
	d1, err := a.GetTaskflow("examples.taskflows.echo")
	require.NoError(t, err)
	d2, err := a.GetTaskflow("examples.taskflows.echo")
	require.NoError(t, err)
	require.Same(t, d1, d2)
}
