// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package capi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// openAIChatPrefixes are the OpenAI model-id prefixes assumed to support
// tool calls. The OpenAI /v1/models catalog exposes no capability metadata,
// so we rely on a prefix allowlist of known chat-completion families.
var openAIChatPrefixes = []string{"gpt-3.5", "gpt-4", "gpt-5", "o1", "o3", "o4", "chatgpt-"}

// parseModelsList extracts the models list from a catalog response body, which
// is either a bare JSON array or an object with a "data" array.
func parseModelsList(body any) []map[string]any {
	var out []map[string]any
	switch v := body.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
	case map[string]any:
		if data, ok := v["data"].([]any); ok {
			for _, item := range data {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// checkToolCalls reports whether model supports tool calls according to its
// catalog entry, using provider-specific rules.
func (p Provider) checkToolCalls(model string, info map[string]any) bool {
	switch p.Name {
	case "copilot":
		caps, _ := info["capabilities"].(map[string]any)
		supports, _ := caps["supports"].(map[string]any)
		ok, _ := supports["tool_calls"].(bool)
		return ok
	case "github-models":
		caps, _ := info["capabilities"].([]any)
		for _, c := range caps {
			if s, ok := c.(string); ok && s == "tool-calling" {
				return true
			}
		}
		return false
	case "openai":
		id, _ := info["id"].(string)
		id = strings.ToLower(id)
		for _, prefix := range openAIChatPrefixes {
			if strings.HasPrefix(id, prefix) {
				return true
			}
		}
		return false
	default:
		// Optimistically assume support when present in the catalog.
		return len(info) > 0
	}
}

// catalogURL joins the provider's catalog path against its base URL authority.
func (p Provider) catalogURL() (string, error) {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(p.ModelsCatalog)
	if err != nil {
		return "", err
	}
	return u.ResolveReference(ref).String(), nil
}

// ListCapiModels retrieves the available models from the configured API
// endpoint, keyed by model id. Network or decoding failures are logged and
// yield an empty map, mirroring the Python implementation.
func ListCapiModels(token, endpoint string) map[string]map[string]any {
	provider := GetProvider(endpoint)
	models := map[string]map[string]any{}

	catalog, err := provider.catalogURL()
	if err != nil {
		log.Printf("failed to build model catalog URL for %s: %v", provider.BaseURL, err)
		return models
	}

	req, err := http.NewRequest(http.MethodGet, catalog, nil)
	if err != nil {
		log.Printf("failed to list models from %s: %v", provider.BaseURL, err)
		return models
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	for k, v := range provider.ExtraHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to list models from %s: %v", provider.BaseURL, err)
		return models
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("failed to list models from %s: unexpected status %s", provider.BaseURL, resp.Status)
		return models
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to list models from %s: %v", provider.BaseURL, err)
		return models
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		log.Printf("failed to list models from %s: %v", provider.BaseURL, err)
		return models
	}

	for _, m := range parseModelsList(decoded) {
		if id, ok := m["id"].(string); ok {
			models[id] = m
		}
	}
	return models
}

// SupportsToolCalls reports whether model supports tool calls given a catalog
// map as returned by ListCapiModels.
func SupportsToolCalls(model string, models map[string]map[string]any, endpoint string) bool {
	provider := GetProvider(endpoint)
	return provider.checkToolCalls(model, models[model])
}

// ListToolCallModels returns the sorted ids of models that support tool calls.
func ListToolCallModels(token, endpoint string) []string {
	provider := GetProvider(endpoint)
	models := ListCapiModels(token, endpoint)
	var ids []string
	for id, info := range models {
		if provider.checkToolCalls(id, info) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
