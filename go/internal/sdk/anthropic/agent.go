// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package anthropic implements the Anthropic backend adapter on top of the
// anthropics/anthropic-sdk-go SDK. It drives the native Messages API
// (/v1/messages) through a hand-rolled streaming agent loop with MCP tool
// calling, extended thinking, and prompt caching, porting the Python
// anthropic_sdk backend. It registers itself as the "anthropic_sdk" backend.
//
// Auth note: the Anthropic SDK sends an x-api-key header by default, but
// providers that use Bearer auth (see capi.Provider.BearerAuth) need
// Authorization: Bearer instead. For those providers we inject the bearer
// header explicitly and hand the SDK a placeholder api key so it does not
// also leak the real token via x-api-key.
package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/capi"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// defaultMaxTokens is the fallback max_tokens when the model config does not
// specify one, matching the Python anthropic_sdk backend.
const defaultMaxTokens = 16384

func init() {
	sdk.Register(&Backend{})
}

// Backend is the Anthropic SDK adapter.
type Backend struct{}

// Name returns the backend identifier.
func (*Backend) Name() string { return "anthropic_sdk" }

// Validate rejects spec features the anthropic backend cannot honour:
// multi-personality handoffs (no Messages-API equivalent) and a missing
// model. Surfacing these at validation time matches the Python
// BackendCapabilityError / BackendBadRequestError behaviour.
func (*Backend) Validate(spec *sdk.AgentSpec) error {
	if len(spec.Handoffs) > 0 || spec.InHandoffGraph {
		return &sdk.CapabilityError{Msg: "anthropic_sdk backend (Go) does not support multi-personality handoffs"}
	}
	if spec.Model == "" {
		return &sdk.BadRequestError{Msg: "anthropic_sdk: model is required"}
	}
	return nil
}

// agent is the backend-native agent handle. It holds the Anthropic client,
// the converted tool definitions, and the connected MCP servers used to
// dispatch tool calls. Anthropic returns namespace-prefixed tool names in
// tool_use blocks, so dispatch routes them back to the owning server.
type agent struct {
	client         anthropic.Client
	model          string
	system         string
	maxTokens      int64
	tools          []anthropic.ToolUnionParam
	servers        []*mcp.Server
	settings       map[string]any
	streamThinking bool
	exclude        bool
}

// Close releases the agent. The anthropic-go client uses the default HTTP
// pool, so there is nothing to free explicitly.
func (a *agent) Close(_ context.Context) error { return nil }

// Build constructs an anthropic agent: it resolves the endpoint/token, applies
// the provider auth scheme, lists MCP tools, applies the blocked-tools filter,
// and converts tool schemas into the Anthropic tool format.
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
		option.WithHTTPClient(&http.Client{Timeout: 300 * time.Second}),
	}
	for k, v := range provider.ExtraHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}
	// Providers with BearerAuth need Authorization: Bearer instead of the
	// Anthropic SDK's native x-api-key header. Use a placeholder api key so
	// the SDK does not also send the real token via x-api-key. Endpoints not
	// in the provider registry default to native SDK auth.
	if provider.BearerAuth {
		opts = append(opts,
			option.WithHeader("Authorization", "Bearer "+token),
			option.WithAPIKey("placeholder"),
		)
	} else {
		opts = append(opts, option.WithAPIKey(token))
	}
	client := anthropic.NewClient(opts...)

	a := &agent{
		client:         client,
		model:          spec.Model,
		system:         spec.Instructions,
		maxTokens:      resolveMaxTokens(spec.ModelSettings),
		settings:       spec.ModelSettings,
		streamThinking: boolSetting(spec.ModelSettings, "stream_thinking"),
		exclude:        spec.ExcludeFromContext,
	}

	blocked := toSet(spec.BlockedTools)
	for _, ms := range spec.MCPServers {
		srv, ok := ms.Native.(*mcp.Server)
		if !ok {
			return nil, fmt.Errorf("anthropic backend: MCP server %q has no native handle", ms.Name)
		}
		a.servers = append(a.servers, srv)
		tools, err := srv.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing tools for %q: %w", ms.Name, err)
		}
		for _, t := range tools {
			if blocked[srv.BareToolName(t.Name)] {
				continue
			}
			a.tools = append(a.tools, toToolParam(t))
		}
	}
	return a, nil
}

// resolveToken returns the API token, honouring a per-model env var name when
// given (matching the Python "token:" model setting).
func resolveToken(envName string) (string, error) {
	if envName != "" {
		v := os.Getenv(envName)
		if v == "" {
			return "", &sdk.BadRequestError{Msg: fmt.Sprintf("anthropic_sdk: token env var %q is not set", envName)}
		}
		return v, nil
	}
	token, err := capi.Token()
	if err != nil {
		return "", &sdk.BadRequestError{Msg: "anthropic_sdk: no API token available (" + err.Error() + ")"}
	}
	return token, nil
}

// toToolParam converts an MCP tool into an Anthropic custom tool definition.
func toToolParam(t mcp.Tool) anthropic.ToolUnionParam {
	schema := anthropic.ToolInputSchemaParam{}
	if m, ok := t.InputSchema.(map[string]any); ok {
		if props, ok := m["properties"]; ok {
			schema.Properties = props
		}
		if req, ok := stringSlice(m["required"]); ok {
			schema.Required = req
		}
	}
	tool := anthropic.ToolParam{
		Name:        t.Name,
		InputSchema: schema,
	}
	if t.Description != "" {
		tool.Description = anthropic.String(t.Description)
	}
	return anthropic.ToolUnionParam{OfTool: &tool}
}

// resolveMaxTokens reads max_tokens from model_settings, defaulting when unset.
func resolveMaxTokens(settings map[string]any) int64 {
	if v, ok := settings["max_tokens"]; ok {
		if n, ok := toInt64(v); ok {
			return n
		}
	}
	return defaultMaxTokens
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func boolSetting(settings map[string]any, key string) bool {
	if v, ok := settings[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func stringSlice(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
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
