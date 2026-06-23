// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Command seclab-taskflow-agent is the Go implementation of the SecLab
// Taskflow Agent: an MCP-enabled, YAML-driven agentic workflow runner.
package main

import (
	"os"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
