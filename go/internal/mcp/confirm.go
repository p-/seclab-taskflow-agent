// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// confirmToolCall interactively prompts the user to allow a tool call,
// porting “MCPNamespaceWrap.confirm_tool“. It returns true on approval.
func confirmToolCall(toolName string, args map[string]any) bool {
	var rendered []string
	for k, v := range args {
		b, _ := json.Marshal(v)
		rendered = append(rendered, fmt.Sprintf("%s=%s", k, string(b)))
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("** \U0001F916\u2757 Allow tool call?: %s(%s) (yes/no): ", toolName, strings.Join(rendered, ","))
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		switch strings.TrimSpace(line) {
		case "yes", "y":
			return true
		case "no", "n":
			return false
		}
	}
}

// headerRoundTripper injects static headers into every request.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}

// httpClientWithHeaders returns an *http.Client that adds the given headers
// to every request. A nil/empty header map yields the default client.
func httpClientWithHeaders(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &headerRoundTripper{base: http.DefaultTransport, headers: headers},
	}
}
