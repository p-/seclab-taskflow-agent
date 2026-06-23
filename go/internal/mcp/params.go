// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package mcp

import (
	"fmt"
	"os"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/tmpl"
)

// ToolboxStore is the minimal loader interface needed to resolve toolboxes.
type ToolboxStore interface {
	GetToolbox(name string) (*grammar.ToolboxDocument, error)
	tmpl.PromptStore
}

// ResolvedParams holds the connection parameters for one toolbox after
// template expansion, ready to drive a transport.
type ResolvedParams struct {
	Toolbox              string
	Kind                 string // "stdio" | "streamable" | "sse"
	Reconnecting         bool
	Command              string
	Args                 []string
	Env                  map[string]string
	URL                  string
	Headers              map[string]string
	Timeout              float64
	Confirm              []string
	ServerPrompt         string
	ClientSessionTimeout float64
}

// proxyVars are forwarded from the parent environment into stdio MCP server
// subprocesses so HTTP clients work inside network-isolated environments.
var proxyVars = []string{
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "no_proxy",
}

// ResolveParams resolves the requested toolboxes into connection parameters,
// expanding “{{ env(...) }}“ references in env, args, and headers. It ports
// the Python “mcp_client_params“.
func ResolveParams(store ToolboxStore, toolboxes []string) ([]ResolvedParams, error) {
	out := make([]ResolvedParams, 0, len(toolboxes))
	for _, tb := range toolboxes {
		doc, err := store.GetToolbox(tb)
		if err != nil {
			return nil, err
		}
		sp := doc.ServerParams
		rp := ResolvedParams{
			Toolbox:              tb,
			Kind:                 sp.Kind,
			Reconnecting:         sp.Reconnecting,
			Confirm:              doc.Confirm,
			ServerPrompt:         doc.ServerPrompt,
			ClientSessionTimeout: doc.ClientSessionTimeout,
			Timeout:              sp.Timeout,
		}

		switch sp.Kind {
		case "stdio":
			env, err := expandEnv(store, sp.Env)
			if err != nil {
				return nil, err
			}
			if env != nil {
				for _, pv := range proxyVars {
					if _, ok := env[pv]; !ok {
						if v, ok := os.LookupEnv(pv); ok {
							env[pv] = v
						}
					}
				}
			}
			args, err := expandArgs(store, sp.Args)
			if err != nil {
				return nil, err
			}
			rp.Command = sp.Command
			rp.Args = args
			rp.Env = env

		case "streamable":
			headers, err := resolveHeaders(store, sp.Headers, sp.OptionalHeaders)
			if err != nil {
				return nil, err
			}
			rp.URL = sp.URL
			rp.Headers = headers
			if sp.Command != "" {
				env, err := expandEnv(store, sp.Env)
				if err != nil {
					return nil, err
				}
				args, err := expandArgs(store, sp.Args)
				if err != nil {
					return nil, err
				}
				rp.Command = sp.Command
				rp.Args = args
				rp.Env = env
			}

		case "sse":
			headers, err := resolveHeaders(store, sp.Headers, sp.OptionalHeaders)
			if err != nil {
				return nil, err
			}
			rp.URL = sp.URL
			rp.Headers = headers

		default:
			return nil, fmt.Errorf("unsupported MCP transport %q", sp.Kind)
		}

		out = append(out, rp)
	}
	return out, nil
}

// expandEnv expands template expressions in env values. A value whose
// required env var is missing is dropped (matching the Python behaviour of
// assuming a default configuration is available).
func expandEnv(store tmpl.PromptStore, env map[string]string) (map[string]string, error) {
	if env == nil {
		return nil, nil
	}
	out := map[string]string{}
	for k, v := range env {
		rendered, err := tmpl.SwapEnv(v, nil)
		if err != nil {
			continue
		}
		out[k] = rendered
	}
	return out, nil
}

func expandArgs(store tmpl.PromptStore, args []string) ([]string, error) {
	if args == nil {
		return nil, nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		rendered, err := tmpl.SwapEnv(a, nil)
		if err != nil {
			return nil, err
		}
		out[i] = rendered
	}
	return out, nil
}

// resolveHeaders expands and merges required and optional headers. Required
// headers fail on missing env vars; optional headers are dropped silently.
func resolveHeaders(store tmpl.PromptStore, headers, optional map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range headers {
		rendered, err := tmpl.SwapEnv(v, nil)
		if err != nil {
			return nil, err
		}
		out[k] = rendered
	}
	for k, v := range optional {
		rendered, err := tmpl.SwapEnv(v, nil)
		if err != nil {
			continue
		}
		out[k] = rendered
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
