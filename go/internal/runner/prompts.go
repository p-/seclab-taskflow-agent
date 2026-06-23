// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/render"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/tmpl"
)

// buildPromptsToRun builds the list of prompts to execute for a task. For a
// regular task it returns a single rendered prompt. For a repeat_prompt task
// it parses the last MCP tool result as an iterable and renders one prompt per
// element. It ports the Python “_build_prompts_to_run“ and mutates
// lastResults by consuming the last entry on success.
func buildPromptsToRun(
	at *loader.AvailableTools,
	taskPrompt string,
	repeatPrompt bool,
	lastResults *[]string,
	globals map[string]any,
	inputs map[string]any,
) ([]string, error) {
	if !repeatPrompt {
		return []string{taskPrompt}, nil
	}

	if !strings.Contains(strings.ToLower(taskPrompt), "result") {
		render.Output("** \U0001F916\u2757 repeat_prompt enabled but no {{ result }} in prompt\n")
	}
	if len(*lastResults) == 0 {
		return nil, fmt.Errorf("no last MCP tool result available for repeat_prompt")
	}
	last := (*lastResults)[len(*lastResults)-1]

	// The stored result is a JSON envelope {"type":"text","text":"..."}.
	var envelope struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(last), &envelope); err != nil {
		return nil, fmt.Errorf("tool result is not valid JSON: %w", err)
	}

	var items []any
	if err := json.Unmarshal([]byte(envelope.Text), &items); err != nil {
		return nil, fmt.Errorf("result text is not a valid JSON array: %w", err)
	}

	var prompts []string
	if len(items) == 0 {
		render.Output("** \U0001F916\u2757 MCP tool result iterable is empty!\n")
	}
	for _, item := range items {
		rendered, err := tmpl.Render(at, taskPrompt, tmpl.RenderContext{
			Globals: globals,
			Inputs:  inputs,
			Result:  item,
		})
		if err != nil {
			return nil, fmt.Errorf("template rendering failed: %w", err)
		}
		prompts = append(prompts, rendered)
	}

	// Consume only after all prompts rendered successfully.
	*lastResults = (*lastResults)[:len(*lastResults)-1]
	return prompts, nil
}
