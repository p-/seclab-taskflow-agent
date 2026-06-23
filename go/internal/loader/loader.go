// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package loader loads, validates, and caches the YAML grammar files,
// porting the Python “available_tools“ module. Dotted resource names are
// resolved against a search path with an embedded-snapshot fallback.
package loader

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
)

// AvailableTools loads, validates, and caches YAML grammar files as typed
// document structs. It is safe for concurrent use.
type AvailableTools struct {
	mu    sync.Mutex
	cache map[string]map[string]any // filetype -> dotted name -> *Document
}

// New returns an empty AvailableTools registry.
func New() *AvailableTools {
	return &AvailableTools{cache: map[string]map[string]any{}}
}

// rawHeader peeks at the document header to validate the filetype before a
// full decode.
type rawHeader struct {
	Header grammar.Header `yaml:"seclab-taskflow-agent"`
}

func (a *AvailableTools) getCached(filetype, name string) (any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if m, ok := a.cache[filetype]; ok {
		if v, ok := m[name]; ok {
			return v, true
		}
	}
	return nil, false
}

func (a *AvailableTools) putCached(filetype, name string, v any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cache[filetype] == nil {
		a.cache[filetype] = map[string]any{}
	}
	a.cache[filetype][name] = v
}

// load resolves, validates, and decodes a dotted resource into out, which
// must be a pointer to a document struct. expectedType is the filetype the
// header must declare.
func (a *AvailableTools) load(expectedType, name string, out any) error {
	data, source, err := resolve(name)
	if err != nil {
		return err
	}

	var hdr rawHeader
	if err := yaml.Unmarshal(data, &hdr); err != nil {
		return fmt.Errorf("error in %s: %w", source, err)
	}
	if hdr.Header.Filetype != expectedType {
		return fmt.Errorf("error in %s: expected filetype %q, got %q", source, expectedType, hdr.Header.Filetype)
	}

	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("validation error loading %s: %w", name, err)
	}
	return nil
}

// GetTaskflow loads and validates a taskflow document.
func (a *AvailableTools) GetTaskflow(name string) (*grammar.TaskflowDocument, error) {
	if v, ok := a.getCached(grammar.FiletypeTaskflow, name); ok {
		return v.(*grammar.TaskflowDocument), nil
	}
	doc := &grammar.TaskflowDocument{}
	if err := a.load(grammar.FiletypeTaskflow, name, doc); err != nil {
		return nil, err
	}
	a.putCached(grammar.FiletypeTaskflow, name, doc)
	return doc, nil
}

// GetPersonality loads and validates a personality document.
func (a *AvailableTools) GetPersonality(name string) (*grammar.PersonalityDocument, error) {
	if v, ok := a.getCached(grammar.FiletypePersonality, name); ok {
		return v.(*grammar.PersonalityDocument), nil
	}
	doc := &grammar.PersonalityDocument{}
	if err := a.load(grammar.FiletypePersonality, name, doc); err != nil {
		return nil, err
	}
	a.putCached(grammar.FiletypePersonality, name, doc)
	return doc, nil
}

// GetToolbox loads and validates a toolbox document.
func (a *AvailableTools) GetToolbox(name string) (*grammar.ToolboxDocument, error) {
	if v, ok := a.getCached(grammar.FiletypeToolbox, name); ok {
		return v.(*grammar.ToolboxDocument), nil
	}
	doc := &grammar.ToolboxDocument{}
	if err := a.load(grammar.FiletypeToolbox, name, doc); err != nil {
		return nil, err
	}
	a.putCached(grammar.FiletypeToolbox, name, doc)
	return doc, nil
}

// GetModelConfig loads and validates a model_config document.
func (a *AvailableTools) GetModelConfig(name string) (*grammar.ModelConfigDocument, error) {
	if v, ok := a.getCached(grammar.FiletypeModelConfig, name); ok {
		return v.(*grammar.ModelConfigDocument), nil
	}
	doc := &grammar.ModelConfigDocument{}
	if err := a.load(grammar.FiletypeModelConfig, name, doc); err != nil {
		return nil, err
	}
	a.putCached(grammar.FiletypeModelConfig, name, doc)
	return doc, nil
}

// GetPrompt loads and validates a reusable prompt document.
func (a *AvailableTools) GetPrompt(name string) (*grammar.PromptDocument, error) {
	if v, ok := a.getCached(grammar.FiletypePrompt, name); ok {
		return v.(*grammar.PromptDocument), nil
	}
	doc := &grammar.PromptDocument{}
	if err := a.load(grammar.FiletypePrompt, name, doc); err != nil {
		return nil, err
	}
	a.putCached(grammar.FiletypePrompt, name, doc)
	return doc, nil
}
