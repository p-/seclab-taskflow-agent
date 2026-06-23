// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package tmpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
)

// fakeStore returns a fixed prompt for any include lookup.
type fakeStore struct{ prompt string }

func (f fakeStore) GetPrompt(string) (*grammar.PromptDocument, error) {
	return &grammar.PromptDocument{Prompt: f.prompt}, nil
}

func TestRenderGlobalsInputsResult(t *testing.T) {
	out, err := Render(fakeStore{}, "{{ globals.topic }} {{ inputs.format }} {{ result.name }}",
		RenderContext{
			Globals: map[string]any{"topic": "fruit"},
			Inputs:  map[string]any{"format": "short"},
			Result:  map[string]any{"name": "apple"},
		})
	require.NoError(t, err)
	require.Equal(t, "fruit short apple", out)
}

func TestRenderEnvFunction(t *testing.T) {
	t.Setenv("MY_VAR", "hello")
	out, err := Render(fakeStore{}, "{{ env('MY_VAR') }}", RenderContext{})
	require.NoError(t, err)
	require.Equal(t, "hello", out)
}

func TestRenderEnvDefaultAndRequired(t *testing.T) {
	out, err := Render(fakeStore{}, "{{ env('NOPE', 'fallback') }}", RenderContext{})
	require.NoError(t, err)
	require.Equal(t, "fallback", out)

	_, err = Render(fakeStore{}, "{{ env('DEFINITELY_MISSING_VAR') }}", RenderContext{})
	require.Error(t, err)
}

func TestRenderStrictUndefined(t *testing.T) {
	_, err := Render(fakeStore{}, "{{ globals.missing.attr }}", RenderContext{Globals: map[string]any{}})
	require.Error(t, err)
}

func TestRenderInclude(t *testing.T) {
	out, err := Render(fakeStore{prompt: "INCLUDED:{{ globals.x }}"},
		"top {% include 'examples.prompts.foo' %}",
		RenderContext{Globals: map[string]any{"x": "Y"}})
	require.NoError(t, err)
	require.Contains(t, out, "INCLUDED:Y")
}

func TestSwapEnv(t *testing.T) {
	t.Setenv("LOG_DIR", "/tmp/logs")
	out, err := SwapEnv("{{ env('LOG_DIR') }}", nil)
	require.NoError(t, err)
	require.Equal(t, "/tmp/logs", out)
}
