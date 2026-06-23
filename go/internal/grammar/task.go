// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package grammar

import (
	"errors"

	"gopkg.in/yaml.v3"
)

// TaskDefinition is a single task within a taskflow. It captures every field
// the engine recognises; unknown keys are preserved in Extra for
// forward-compatibility and reusable-task merging.
type TaskDefinition struct {
	Name               string            `yaml:"name"`
	Description        string            `yaml:"description"`
	Agents             []string          `yaml:"agents"`
	UserPrompt         string            `yaml:"user_prompt"`
	Run                string            `yaml:"run"`
	Model              string            `yaml:"model"`
	ModelSettings      map[string]any    `yaml:"model_settings"`
	MustComplete       bool              `yaml:"must_complete"`
	Headless           bool              `yaml:"headless"`
	RepeatPrompt       bool              `yaml:"repeat_prompt"`
	ExcludeFromContext bool              `yaml:"exclude_from_context"`
	BlockedTools       []string          `yaml:"blocked_tools"`
	Toolboxes          []string          `yaml:"toolboxes"`
	Env                map[string]string `yaml:"env"`
	Inputs             map[string]any    `yaml:"inputs"`
	MaxSteps           int               `yaml:"max_steps"`
	Uses               string            `yaml:"uses"`
	AsyncTask          bool              `yaml:"async"`
	AsyncLimit         int               `yaml:"async_limit"`

	// Extra captures any unrecognised keys (Pydantic ``extra="allow"``).
	Extra map[string]any `yaml:"-"`
}

// taskAlias avoids infinite recursion in UnmarshalYAML while still letting us
// post-process defaults and capture extra keys.
type taskAlias TaskDefinition

// UnmarshalYAML decodes a task, applies the AsyncLimit default, captures
// unknown keys into Extra, and enforces the run/user_prompt exclusivity rule.
func (t *TaskDefinition) UnmarshalYAML(value *yaml.Node) error {
	alias := taskAlias{AsyncLimit: 5}
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*t = TaskDefinition(alias)

	// Capture extra keys not covered by the known fields.
	var all map[string]any
	if err := value.Decode(&all); err != nil {
		return err
	}
	known := knownTaskKeys()
	t.Extra = map[string]any{}
	for k, v := range all {
		if !known[k] {
			t.Extra[k] = v
		}
	}
	if len(t.Extra) == 0 {
		t.Extra = nil
	}

	return t.validate()
}

func (t *TaskDefinition) validate() error {
	if t.Run != "" && t.UserPrompt != "" {
		return errors.New("shell task ('run') and prompt task ('user_prompt') are mutually exclusive")
	}
	return nil
}

func knownTaskKeys() map[string]bool {
	return map[string]bool{
		"name": true, "description": true, "agents": true, "user_prompt": true,
		"run": true, "model": true, "model_settings": true, "must_complete": true,
		"headless": true, "repeat_prompt": true, "exclude_from_context": true,
		"blocked_tools": true, "toolboxes": true, "env": true, "inputs": true,
		"max_steps": true, "uses": true, "async": true, "async_limit": true,
	}
}

// TaskWrapper wraps the “- task:“ YAML list entry.
type TaskWrapper struct {
	Task TaskDefinition `yaml:"task"`
}
