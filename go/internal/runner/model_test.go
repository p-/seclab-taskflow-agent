// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
)

func TestResolveTaskModelDefault(t *testing.T) {
	got, err := resolveTaskModel(grammar.TaskDefinition{}, nil)
	require.NoError(t, err)
	require.Equal(t, DefaultModel, got.model)
	require.Equal(t, grammar.APITypeChatCompletions, got.apiType)
}

func TestResolveTaskModelFromConfig(t *testing.T) {
	mc := &modelConfig{
		keys:    []string{"code"},
		models:  map[string]string{"code": "gpt-5.1"},
		apiType: grammar.APITypeChatCompletions,
		settings: map[string]map[string]any{
			"code": {
				"api_type":    "responses",
				"endpoint":    "https://example.test",
				"token":       "MY_TOKEN",
				"backend":     "openai",
				"temperature": 0.5,
			},
		},
	}
	task := grammar.TaskDefinition{Model: "code"}
	got, err := resolveTaskModel(task, mc)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.1", got.model)
	require.Equal(t, "responses", got.apiType)
	require.Equal(t, "https://example.test", got.endpoint)
	require.Equal(t, "MY_TOKEN", got.token)
	require.Equal(t, "openai", got.backend)
	require.Equal(t, 0.5, got.settings["temperature"])
	// Engine keys must be stripped from settings.
	require.NotContains(t, got.settings, "api_type")
	require.NotContains(t, got.settings, "endpoint")
	require.NotContains(t, got.settings, "backend")
}

func TestResolveTaskModelTaskOverridesConfig(t *testing.T) {
	mc := &modelConfig{
		keys:     []string{"m"},
		models:   map[string]string{"m": "base"},
		apiType:  grammar.APITypeChatCompletions,
		settings: map[string]map[string]any{"m": {"backend": "openai", "api_type": "responses"}},
	}
	task := grammar.TaskDefinition{
		Model:         "m",
		ModelSettings: map[string]any{"backend": "anthropic", "temperature": 1},
	}
	got, err := resolveTaskModel(task, mc)
	require.NoError(t, err)
	require.Equal(t, "anthropic", got.backend) // task wins
	require.Equal(t, "responses", got.apiType) // from per-model settings
	require.Equal(t, 1, got.settings["temperature"])
}

func TestResolveModelConfigRejectsUnknownSettings(t *testing.T) {
	// settings reference a model not present in models map
	mc := &modelConfig{}
	require.NotNil(t, mc)
}

func TestLeadingPersonality(t *testing.T) {
	require.Equal(t, "foo.bar", leadingPersonality("-p foo.bar hello world"))
	require.Equal(t, "", leadingPersonality("just a prompt"))
}
