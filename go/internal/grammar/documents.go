// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package grammar

import "gopkg.in/yaml.v3"

// Valid API type values for model configuration.
const (
	APITypeChatCompletions = "chat_completions"
	APITypeResponses       = "responses"
	APITypeMessages        = "messages"
)

// TaskflowDocument is a complete taskflow YAML document.
type TaskflowDocument struct {
	Header         Header         `yaml:"seclab-taskflow-agent"`
	Globals        map[string]any `yaml:"globals"`
	ModelConfigRef string         `yaml:"model_config"`
	Taskflow       []TaskWrapper  `yaml:"taskflow"`
}

// PersonalityDocument is a personality YAML document.
type PersonalityDocument struct {
	Header      Header   `yaml:"seclab-taskflow-agent"`
	Personality string   `yaml:"personality"`
	Task        string   `yaml:"task"`
	Toolboxes   []string `yaml:"toolboxes"`
}

// ServerParams holds the MCP server connection parameters inside a toolbox.
type ServerParams struct {
	Kind            string            `yaml:"kind"`
	Command         string            `yaml:"command"`
	Args            []string          `yaml:"args"`
	Env             map[string]string `yaml:"env"`
	URL             string            `yaml:"url"`
	Headers         map[string]string `yaml:"headers"`
	OptionalHeaders map[string]string `yaml:"optional_headers"`
	Timeout         float64           `yaml:"timeout"`
	Reconnecting    bool              `yaml:"reconnecting"`
}

// ToolboxDocument is a toolbox YAML document defining an MCP server config.
type ToolboxDocument struct {
	Header               Header       `yaml:"seclab-taskflow-agent"`
	ServerParams         ServerParams `yaml:"server_params"`
	ServerPrompt         string       `yaml:"server_prompt"`
	Confirm              []string     `yaml:"confirm"`
	ClientSessionTimeout float64      `yaml:"client_session_timeout"`
}

// ModelConfigDocument maps logical model names to provider IDs.
type ModelConfigDocument struct {
	Header        Header                    `yaml:"seclab-taskflow-agent"`
	APIType       string                    `yaml:"api_type"`
	Backend       string                    `yaml:"backend"`
	Models        map[string]string         `yaml:"models"`
	ModelSettings map[string]map[string]any `yaml:"model_settings"`
}

// UnmarshalYAML applies the default api_type of “chat_completions“.
func (m *ModelConfigDocument) UnmarshalYAML(value *yaml.Node) error {
	type alias ModelConfigDocument
	a := alias{APIType: APITypeChatCompletions}
	if err := value.Decode(&a); err != nil {
		return err
	}
	*m = ModelConfigDocument(a)
	if m.APIType == "" {
		m.APIType = APITypeChatCompletions
	}
	return nil
}

// PromptDocument is a reusable prompt YAML document.
type PromptDocument struct {
	Header Header `yaml:"seclab-taskflow-agent"`
	Prompt string `yaml:"prompt"`
}
