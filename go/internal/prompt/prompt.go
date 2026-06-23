// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package prompt assembles the agent system prompt from a personality, task,
// guidelines, and MCP server prompts. It ports the Python “mcp_prompt“
// module.
package prompt

import "strings"

// BuildSystemPrompt builds a structured system prompt. Each optional section
// is appended only when its slice is non-empty, matching the Python layout.
func BuildSystemPrompt(systemPrompt, task string, importantGuidelines, serverPrompts []string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(systemPrompt)
	b.WriteString("\n")

	if len(importantGuidelines) > 0 {
		b.WriteString("\n\n# Important Guidelines\n\n")
		for i, g := range importantGuidelines {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("- IMPORTANT: ")
			b.WriteString(g)
		}
		b.WriteString("\n")
	}

	if len(serverPrompts) > 0 {
		b.WriteString("\n\n# Additional Guidelines\n\n")
		b.WriteString(strings.Join(serverPrompts, "\n\n"))
		b.WriteString("\n\n")
	}

	if task != "" {
		b.WriteString("\n\n# Primary Task to Complete\n\n")
		b.WriteString(task)
		b.WriteString("\n\n")
	}

	return b.String()
}
