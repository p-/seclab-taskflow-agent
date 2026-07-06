// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package capi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseModelsList(t *testing.T) {
	// Bare array (GitHub Models style).
	arr := []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
	if got := parseModelsList(arr); len(got) != 2 {
		t.Fatalf("bare array: got %d models, want 2", len(got))
	}
	// {"data": [...]} wrapper (OpenAI/Copilot style).
	obj := map[string]any{"data": []any{map[string]any{"id": "a"}}}
	if got := parseModelsList(obj); len(got) != 1 {
		t.Fatalf("data wrapper: got %d models, want 1", len(got))
	}
	// Unknown shape.
	if got := parseModelsList("nope"); got != nil {
		t.Fatalf("unknown shape: got %v, want nil", got)
	}
}

func TestCheckToolCalls(t *testing.T) {
	copilot := providers()["api.githubcopilot.com"]
	yes := map[string]any{"capabilities": map[string]any{"supports": map[string]any{"tool_calls": true}}}
	no := map[string]any{"capabilities": map[string]any{"supports": map[string]any{"tool_calls": false}}}
	if !copilot.checkToolCalls("m", yes) || copilot.checkToolCalls("m", no) {
		t.Fatal("copilot tool_calls capability check failed")
	}

	ghm := providers()["models.github.ai"]
	if !ghm.checkToolCalls("m", map[string]any{"capabilities": []any{"tool-calling"}}) {
		t.Fatal("github-models should detect tool-calling")
	}
	if ghm.checkToolCalls("m", map[string]any{"capabilities": []any{"streaming"}}) {
		t.Fatal("github-models should reject non tool-calling")
	}

	oai := providers()["api.openai.com"]
	if !oai.checkToolCalls("m", map[string]any{"id": "gpt-4o"}) {
		t.Fatal("openai should allowlist gpt-4o")
	}
	if oai.checkToolCalls("m", map[string]any{"id": "text-embedding-3"}) {
		t.Fatal("openai should reject embeddings")
	}
}

func TestListToolCallModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]any{"id": "beta"},
				map[string]any{"id": "alpha"},
			},
		})
	}))
	defer srv.Close()

	// Unknown host => custom provider, which accepts any non-empty entry.
	got := ListToolCallModels("tok", srv.URL)
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (sorted)", got, want)
	}
}
