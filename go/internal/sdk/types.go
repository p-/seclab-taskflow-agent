// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package sdk defines the backend-neutral types and the Backend interface
// that every model-provider adapter implements. It ports the Python
// “sdk.base“ module. A single backend (openai) ships initially; the
// registry leaves room for additional adapters.
package sdk

import "context"

// StreamEvent is the sum type of events a backend emits while running.
// Implemented by TextDelta and ToolEnd.
type StreamEvent interface{ isStreamEvent() }

// TextDelta is an incremental text chunk emitted while the model generates.
type TextDelta struct{ Text string }

func (TextDelta) isStreamEvent() {}

// ToolEnd reports that a tool call completed; Text is the raw tool result.
type ToolEnd struct {
	ToolName string
	Text     string
}

func (ToolEnd) isStreamEvent() {}

// MCPServerSpec is a backend-neutral descriptor for a connected MCP server.
// The Native field carries the adapter-specific handle (e.g. an *mcp pool
// entry) that the backend uses to list and call tools.
type MCPServerSpec struct {
	Name   string
	Kind   string // "stdio" | "streamable"
	Native any
}

// AgentSpec is a backend-neutral agent configuration.
type AgentSpec struct {
	Name               string
	Instructions       string
	Model              string
	ModelSettings      map[string]any
	MCPServers         []MCPServerSpec
	Handoffs           []*AgentSpec
	ExcludeFromContext bool
	APIType            string
	Endpoint           string
	TokenEnv           string
	InHandoffGraph     bool
	BlockedTools       []string
	Headless           bool
}

// Stream yields neutral stream events from a running agent. Recv returns
// io.EOF when the run completes normally.
type Stream interface {
	Recv() (StreamEvent, error)
	Close() error
}

// Agent is a backend-native agent handle.
type Agent interface {
	// Close releases resources held by the agent.
	Close(ctx context.Context) error
}

// Backend is the contract every model-provider adapter implements.
type Backend interface {
	// Name returns the backend identifier (e.g. "openai").
	Name() string
	// Validate rejects any spec field the backend cannot honour.
	Validate(spec *AgentSpec) error
	// Build constructs a backend-native agent from a neutral spec.
	Build(ctx context.Context, spec *AgentSpec) (Agent, error)
	// RunStreamed runs the agent against prompt, yielding neutral events.
	RunStreamed(ctx context.Context, agent Agent, prompt string, maxTurns int) (Stream, error)
}
