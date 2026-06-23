// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package mcp

import "github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"

// Specs converts the connected pool into backend-neutral MCP server specs.
// Each spec's Native field carries the *Server so the backend can list and
// call tools without depending on this package's concrete types directly.
func (p *Pool) Specs() []sdk.MCPServerSpec {
	specs := make([]sdk.MCPServerSpec, 0, len(p.Servers))
	for _, s := range p.Servers {
		specs = append(specs, sdk.MCPServerSpec{
			Name:   s.Name,
			Kind:   "", // transport kind is not needed by the openai backend
			Native: s,
		})
	}
	return specs
}
