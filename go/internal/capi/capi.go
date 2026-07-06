// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package capi resolves the AI API endpoint, token, and provider-specific
// behaviour (base URL, default model, extra headers). It ports the Python
// “capi“ module's provider registry for the providers the Go MVP needs.
package capi

import (
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func init() {
	// Best-effort .env loading from the working directory, mirroring the
	// Python agent's ``load_dotenv(find_dotenv(usecwd=True))``.
	_ = godotenv.Load()
}

// Provider encapsulates endpoint-specific behaviour.
type Provider struct {
	Name         string
	BaseURL      string
	DefaultModel string
	// ModelsCatalog is the (absolute) path of the provider's model catalog
	// endpoint, joined against the authority of BaseURL.
	ModelsCatalog string
	ExtraHeaders  map[string]string
	// BearerAuth reports whether the provider authenticates with an
	// Authorization: Bearer header (true) rather than a provider-native
	// scheme such as the Anthropic SDK's x-api-key (false). Known providers
	// use Bearer auth; the generic custom fallback uses native SDK auth.
	BearerAuth bool
}

const defaultProviderHost = "api.githubcopilot.com"

// copilotIntegrationID is sent as the Copilot-Integration-Id header to the
// GitHub Copilot endpoint.
func copilotIntegrationID() string {
	if v := os.Getenv("COPILOT_INTEGRATION_ID"); v != "" {
		return v
	}
	return "vscode-chat"
}

// providers returns the built-in provider registry keyed by host.
func providers() map[string]Provider {
	return map[string]Provider{
		"api.githubcopilot.com": {
			Name:          "copilot",
			BaseURL:       "https://api.githubcopilot.com",
			DefaultModel:  "gpt-4.1",
			ModelsCatalog: "/models",
			ExtraHeaders:  map[string]string{"Copilot-Integration-Id": copilotIntegrationID()},
			BearerAuth:    true,
		},
		"models.github.ai": {
			Name:          "github-models",
			BaseURL:       "https://models.github.ai/inference",
			DefaultModel:  "openai/gpt-4.1",
			ModelsCatalog: "/catalog/models",
			BearerAuth:    true,
		},
		"api.openai.com": {
			Name:          "openai",
			BaseURL:       "https://api.openai.com/v1",
			DefaultModel:  "gpt-4.1",
			ModelsCatalog: "/v1/models",
			BearerAuth:    true,
		},
	}
}

// Endpoint returns the configured AI API endpoint URL.
func Endpoint() string {
	if v := os.Getenv("AI_API_ENDPOINT"); v != "" {
		return v
	}
	return providers()[defaultProviderHost].BaseURL
}

// Token returns the AI API token from AI_API_TOKEN or COPILOT_TOKEN.
func Token() (string, error) {
	if v := os.Getenv("AI_API_TOKEN"); v != "" {
		return v, nil
	}
	if v := os.Getenv("COPILOT_TOKEN"); v != "" {
		return v, nil
	}
	return "", &MissingTokenError{}
}

// MissingTokenError indicates no API token was configured.
type MissingTokenError struct{}

func (*MissingTokenError) Error() string {
	return "AI_API_TOKEN environment variable is not set"
}

// GetProvider returns the Provider for the given endpoint URL (or the
// configured endpoint when empty). Unknown hosts yield a generic custom
// provider using the endpoint as the base URL.
func GetProvider(endpoint string) Provider {
	if endpoint == "" {
		endpoint = Endpoint()
	}
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Host
	}
	if p, ok := providers()[host]; ok {
		return p
	}
	// AWF proxy support: AWF_COPILOT_PROXY names the upstream provider whose
	// behaviour the local proxy mirrors.
	if upstream := strings.TrimSpace(os.Getenv("AWF_COPILOT_PROXY")); upstream != "" {
		key := upstream
		if u, err := url.Parse(upstream); err == nil && u.Host != "" {
			key = u.Host
		}
		if p, ok := providers()[key]; ok {
			p.BaseURL = endpoint
			return p
		}
	}
	return Provider{Name: "custom", BaseURL: endpoint, ModelsCatalog: "/models", DefaultModel: "please-set-default-model-via-env"}
}
