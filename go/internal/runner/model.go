// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package runner is the taskflow execution engine. It ports the Python
// “runner“ module: model resolution, reusable-task merging, prompt
// building, agent deployment, and the top-level run/resume loop.
package runner

import (
	"fmt"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
)

// DefaultModel is used when a task does not specify one.
const DefaultModel = "gpt-4.1"

// modelConfig is the resolved model configuration for a taskflow.
type modelConfig struct {
	keys     []string
	models   map[string]string
	settings map[string]map[string]any
	apiType  string
	backend  string
}

// resolveModelConfig loads and validates the model config document.
func resolveModelConfig(at *loader.AvailableTools, ref string) (*modelConfig, error) {
	doc, err := at.GetModelConfig(ref)
	if err != nil {
		return nil, err
	}
	mc := &modelConfig{
		models:   doc.Models,
		settings: doc.ModelSettings,
		apiType:  doc.APIType,
		backend:  doc.Backend,
	}
	if mc.models == nil {
		mc.models = map[string]string{}
	}
	for k := range mc.models {
		mc.keys = append(mc.keys, k)
	}
	for name := range mc.settings {
		if _, ok := mc.models[name]; !ok {
			return nil, fmt.Errorf("model_config %q: settings reference unknown model %q", ref, name)
		}
	}
	if mc.apiType == "" {
		mc.apiType = grammar.APITypeChatCompletions
	}
	return mc, nil
}

// resolvedTaskModel is the result of resolving a task's model fields.
type resolvedTaskModel struct {
	model    string
	settings map[string]any
	apiType  string
	endpoint string
	token    string
	backend  string
}

// resolveTaskModel resolves the final model id, settings, and per-model
// overrides for a task, honouring the precedence in the Python
// “_resolve_task_model“: task model_settings > per-model settings > config
// defaults.
func resolveTaskModel(task grammar.TaskDefinition, mc *modelConfig) (resolvedTaskModel, error) {
	logical := task.Model
	if logical == "" {
		logical = DefaultModel
	}
	settings := map[string]any{}
	apiType := grammar.APITypeChatCompletions
	if mc != nil {
		apiType = mc.apiType
	}

	if mc != nil && contains(mc.keys, logical) {
		if s, ok := mc.settings[logical]; ok {
			for k, v := range s {
				settings[k] = v
			}
		}
		logical = mc.models[logical]
	}

	apiType = popString(settings, "api_type", apiType)
	endpoint := popString(settings, "endpoint", "")
	token := popString(settings, "token", "")
	backend := popString(settings, "backend", "")
	if mc != nil && backend == "" {
		backend = mc.backend
	}

	// Task-level model_settings override engine keys and merge the rest.
	taskSettings := map[string]any{}
	for k, v := range task.ModelSettings {
		taskSettings[k] = v
	}
	apiType = popString(taskSettings, "api_type", apiType)
	endpoint = popString(taskSettings, "endpoint", endpoint)
	token = popString(taskSettings, "token", token)
	backend = popString(taskSettings, "backend", backend)
	for k, v := range taskSettings {
		settings[k] = v
	}

	return resolvedTaskModel{
		model:    logical,
		settings: settings,
		apiType:  apiType,
		endpoint: endpoint,
		token:    token,
		backend:  backend,
	}, nil
}

// mergeReusableTask merges a referenced reusable taskflow's single task into
// the current task, with the current task's explicit fields winning.
func mergeReusableTask(at *loader.AvailableTools, task grammar.TaskDefinition) (grammar.TaskDefinition, error) {
	doc, err := at.GetTaskflow(task.Uses)
	if err != nil {
		return task, fmt.Errorf("no such reusable taskflow %q: %w", task.Uses, err)
	}
	if len(doc.Taskflow) > 1 {
		return task, fmt.Errorf("reusable taskflows can only contain 1 task")
	}
	if len(doc.Taskflow) == 0 {
		return task, fmt.Errorf("reusable taskflow %q is empty", task.Uses)
	}
	parent := doc.Taskflow[0].Task
	return overlayTask(parent, task), nil
}

func popString(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		delete(m, key)
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
