// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package shell executes shell “run:“ tasks. It ports the Python
// “shell_utils“ module: a script is written to a temporary file, executed
// with bash, and the stdout returned as a JSON text-content envelope so the
// result can feed repeat-prompt iteration.
package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// TextContent mirrors the MCP “{"type":"text","text":...}“ envelope that
// the Python agent stores in “last_mcp_tool_results“.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// RunResultJSON executes the script and returns its stdout wrapped as the
// JSON-serialised text content envelope. This matches the exact wire format
// the runner expects for repeat_prompt parsing.
func RunResultJSON(script string) (string, error) {
	stdout, err := run(script)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(TextContent{Type: "text", Text: stdout})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func run(script string) (string, error) {
	f, err := os.CreateTemp("", "seclab-shell-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	cmd := exec.Command("bash", f.Name())
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return "", fmt.Errorf("shell task failed: %w: %s", err, stderr)
	}
	return string(out), nil
}
