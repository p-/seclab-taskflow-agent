// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package openai implements the OpenAI backend adapter on top of the
// openai/openai-go SDK. It drives a hand-rolled streaming agent loop over
// either the Chat Completions or the Responses API (selected per task via the
// api_type field) with MCP tool calling, since the Python default's
// openai-agents SDK has no Go equivalent. It registers itself as the
// "openai" backend.
package openai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/capi"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

func init() {
	sdk.Register(&Backend{})
}

// Backend is the openai SDK adapter.
type Backend struct{}

// Name returns the backend identifier.
func (*Backend) Name() string { return "openai" }

// Validate rejects spec features the openai backend cannot honour:
// multi-personality handoffs, exclude_from_context, and API types other than
// chat_completions or responses. Surfacing these at validation time matches
// the Python BackendCapabilityError behaviour.
func (*Backend) Validate(spec *sdk.AgentSpec) error {
	if len(spec.Handoffs) > 0 {
		return &sdk.CapabilityError{Msg: "openai backend (Go) does not support multi-personality handoffs yet"}
	}
	if spec.ExcludeFromContext {
		return &sdk.CapabilityError{Msg: "openai backend (Go) does not support exclude_from_context yet"}
	}
	switch spec.APIType {
	case "", grammar.APITypeChatCompletions, grammar.APITypeResponses:
		return nil
	default:
		return &sdk.CapabilityError{Msg: fmt.Sprintf("openai backend (Go) only supports api_type chat_completions or responses, got %q", spec.APIType)}
	}
}

// agent is the backend-native agent handle. It holds tool definitions for both
// the Chat Completions and Responses APIs; apiType selects which loop runs.
type agent struct {
	client    oai.Client
	model     string
	system    string
	apiType   string
	tools     []oai.ChatCompletionToolParam
	respTools []responses.ToolUnionParam
	servers   []*mcp.Server
	temp      *float64
	maxTurns  int
}

// Close releases the agent. The openai-go client uses the default HTTP pool,
// so there is nothing to free explicitly.
func (a *agent) Close(_ context.Context) error { return nil }

// Build constructs an openai agent: it resolves the endpoint/token, lists MCP
// tools, applies the blocked-tools filter, and converts tool schemas.
func (b *Backend) Build(ctx context.Context, spec *sdk.AgentSpec) (sdk.Agent, error) {
	endpoint := spec.Endpoint
	if endpoint == "" {
		endpoint = capi.Endpoint()
	}
	token, err := resolveToken(spec.TokenEnv)
	if err != nil {
		return nil, err
	}
	provider := capi.GetProvider(endpoint)

	opts := []option.RequestOption{
		option.WithBaseURL(endpoint),
		option.WithAPIKey(token),
		option.WithHTTPClient(&http.Client{Timeout: 300 * time.Second}),
	}
	for k, v := range provider.ExtraHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}
	client := oai.NewClient(opts...)

	a := &agent{
		client:  client,
		model:   spec.Model,
		system:  spec.Instructions,
		apiType: spec.APIType,
		servers: nil,
	}
	if t := temperatureFromSettings(spec.ModelSettings); t != nil {
		a.temp = t
	}

	blocked := toSet(spec.BlockedTools)
	for _, ms := range spec.MCPServers {
		srv, ok := ms.Native.(*mcp.Server)
		if !ok {
			return nil, fmt.Errorf("openai backend: MCP server %q has no native handle", ms.Name)
		}
		a.servers = append(a.servers, srv)
		tools, err := srv.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing tools for %q: %w", ms.Name, err)
		}
		for _, t := range tools {
			if blocked[srvBareName(srv, t.Name)] {
				continue
			}
			a.tools = append(a.tools, toToolParam(t))
			a.respTools = append(a.respTools, toResponsesToolParam(t))
		}
	}
	return a, nil
}

// resolveToken returns the API token, honouring a per-model env var name when
// given (matching the Python “token:“ model setting).
func resolveToken(envName string) (string, error) {
	if envName != "" {
		v := os.Getenv(envName)
		if v == "" {
			return "", fmt.Errorf("token env var %q is not set", envName)
		}
		return v, nil
	}
	return capi.Token()
}

func toToolParam(t mcp.Tool) oai.ChatCompletionToolParam {
	fn := shared.FunctionDefinitionParam{
		Name:        t.Name,
		Description: oai.String(t.Description),
	}
	if t.InputSchema != nil {
		if m, ok := t.InputSchema.(map[string]any); ok {
			fn.Parameters = shared.FunctionParameters(m)
		}
	}
	return oai.ChatCompletionToolParam{Function: fn}
}

// toResponsesToolParam converts an MCP tool into a Responses API function tool.
func toResponsesToolParam(t mcp.Tool) responses.ToolUnionParam {
	var params map[string]any
	if m, ok := t.InputSchema.(map[string]any); ok {
		params = m
	}
	tool := responses.ToolParamOfFunction(t.Name, params, false)
	if tool.OfFunction != nil {
		tool.OfFunction.Description = oai.String(t.Description)
	}
	return tool
}

func temperatureFromSettings(settings map[string]any) *float64 {
	if settings == nil {
		return nil
	}
	if v, ok := settings["temperature"]; ok {
		switch n := v.(type) {
		case float64:
			return &n
		case int:
			f := float64(n)
			return &f
		}
	}
	return nil
}

func toSet(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	m := make(map[string]bool, len(list))
	for _, s := range list {
		m[s] = true
	}
	return m
}

// srvBareName returns the un-namespaced tool name for blocked-tool matching.
func srvBareName(srv *mcp.Server, namespaced string) string {
	return srv.BareToolName(namespaced)
}
