// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import "github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"

// overlayTask returns parent with any explicitly-set fields from child taking
// precedence. It approximates the Python merge of two “exclude_defaults“
// dumps: a child field that differs from its zero value overrides the parent.
func overlayTask(parent, child grammar.TaskDefinition) grammar.TaskDefinition {
	out := parent
	if child.Name != "" {
		out.Name = child.Name
	}
	if child.Description != "" {
		out.Description = child.Description
	}
	if len(child.Agents) > 0 {
		out.Agents = child.Agents
	}
	if child.UserPrompt != "" {
		out.UserPrompt = child.UserPrompt
	}
	if child.Run != "" {
		out.Run = child.Run
	}
	if child.Model != "" {
		out.Model = child.Model
	}
	if len(child.ModelSettings) > 0 {
		out.ModelSettings = child.ModelSettings
	}
	if child.MustComplete {
		out.MustComplete = child.MustComplete
	}
	if child.Headless {
		out.Headless = child.Headless
	}
	if child.RepeatPrompt {
		out.RepeatPrompt = child.RepeatPrompt
	}
	if child.ExcludeFromContext {
		out.ExcludeFromContext = child.ExcludeFromContext
	}
	if len(child.BlockedTools) > 0 {
		out.BlockedTools = child.BlockedTools
	}
	if len(child.Toolboxes) > 0 {
		out.Toolboxes = child.Toolboxes
	}
	if len(child.Env) > 0 {
		out.Env = child.Env
	}
	if len(child.Inputs) > 0 {
		out.Inputs = child.Inputs
	}
	if child.MaxSteps != 0 {
		out.MaxSteps = child.MaxSteps
	}
	if child.AsyncTask {
		out.AsyncTask = child.AsyncTask
	}
	if child.AsyncLimit != 0 && child.AsyncLimit != 5 {
		out.AsyncLimit = child.AsyncLimit
	}
	// Uses is intentionally cleared so the merged task is not re-merged.
	out.Uses = ""
	return out
}
