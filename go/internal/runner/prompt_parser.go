// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import "strings"

// leadingPersonality extracts a “-p personality“ flag embedded at the start
// of a prompt, porting the minimal behaviour of the Python
// “parse_prompt_args“ used when a task has no explicit agents list. It
// returns the personality name, or "" if none is present.
func leadingPersonality(prompt string) string {
	fields := strings.Fields(prompt)
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == "-p" {
			return fields[i+1]
		}
	}
	return ""
}
