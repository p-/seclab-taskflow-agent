// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package grammar

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHeaderVersionNormalisation(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
		ok   bool
	}{
		{"string", "version: \"1.0\"\nfiletype: taskflow", "1.0", true},
		{"int", "version: 1\nfiletype: taskflow", "1.0", true},
		{"float", "version: 1.0\nfiletype: taskflow", "1.0", true},
		{"unsupported", "version: \"2.0\"\nfiletype: taskflow", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h Header
			err := yaml.Unmarshal([]byte(tc.yaml), &h)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, h.Version)
		})
	}
}

func TestTaskRunPromptExclusive(t *testing.T) {
	src := "run: echo hi\nuser_prompt: do something"
	var task TaskDefinition
	err := yaml.Unmarshal([]byte(src), &task)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestTaskDefaultsAndAlias(t *testing.T) {
	src := `
name: t1
async: true
agents:
  - foo.bar
unknown_key: 42
`
	var task TaskDefinition
	require.NoError(t, yaml.Unmarshal([]byte(src), &task))
	require.True(t, task.AsyncTask)
	require.Equal(t, 5, task.AsyncLimit) // default preserved
	require.Equal(t, []string{"foo.bar"}, task.Agents)
	require.Equal(t, 42, task.Extra["unknown_key"])
}

func TestModelConfigDefaultAPIType(t *testing.T) {
	src := `
seclab-taskflow-agent:
  version: "1.0"
  filetype: model_config
models:
  m1: gpt-4.1
`
	var doc ModelConfigDocument
	require.NoError(t, yaml.Unmarshal([]byte(src), &doc))
	require.Equal(t, APITypeChatCompletions, doc.APIType)
	require.Equal(t, "gpt-4.1", doc.Models["m1"])
}
