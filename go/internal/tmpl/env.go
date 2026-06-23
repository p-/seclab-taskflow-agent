// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package tmpl provides Jinja2-compatible template rendering for taskflow
// prompts, porting the Python “template_utils“ module. It uses gonja for
// Jinja2 syntax compatibility (“{{ globals.x }}“, “{% include %}“,
// “{{ env('VAR') }}“) with strict-undefined semantics.
package tmpl

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
)

// PromptStore is the minimal interface tmpl needs to resolve reusable
// prompts referenced by “{% include 'dotted.path' %}“.
type PromptStore interface {
	GetPrompt(name string) (*grammar.PromptDocument, error)
}

// jinjaConfig mirrors the Python Jinja2 environment: same delimiters,
// trim/lstrip blocks, autoescape off, strict undefined.
func jinjaConfig() *config.Config {
	c := config.New()
	c.VariableStartString = "{{"
	c.VariableEndString = "}}"
	c.BlockStartString = "{%"
	c.BlockEndString = "%}"
	c.AutoEscape = false
	c.TrimBlocks = true
	c.LeftStripBlocks = true
	c.StrictUndefined = true
	return c
}

// environment builds a gonja Environment that registers the “env“ global,
// matching “template_utils.env_function“.
func environment() *exec.Environment {
	ctx := exec.EmptyContext().
		Update(builtins.GlobalFunctions).
		Update(builtins.GlobalVariables)
	ctx.Set("env", envFunction)
	return &exec.Environment{
		Context:           ctx,
		Filters:           builtins.Filters,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods:           builtins.Methods,
	}
}

// envFunction implements the Jinja “env(name, default=None, required=True)“
// helper used in toolbox/prompt templates.
func envFunction(_ *exec.Evaluator, params *exec.VarArgs) *exec.Value {
	if len(params.Args) == 0 {
		return exec.AsValue(fmt.Errorf("env() requires a variable name"))
	}
	name := params.Args[0].String()
	defVal := params.GetKeywordArgument("default", nil)
	required := params.GetKeywordArgument("required", true)
	// Positional default takes precedence: env('VAR', 'fallback').
	if len(params.Args) >= 2 {
		defVal = params.Args[1]
	}

	if v, ok := os.LookupEnv(name); ok {
		return exec.AsValue(v)
	}
	if defVal != nil && !defVal.IsNil() {
		return exec.AsValue(defVal.String())
	}
	if required.Bool() {
		return exec.AsValue(fmt.Errorf("required environment variable %s not found", name))
	}
	return exec.AsValue("")
}

// promptLoader is a gonja Loader that resolves dotted include names through a
// PromptStore. It backs “{% include 'examples.prompts.foo' %}“.
type promptLoader struct {
	store PromptStore
}

func (p *promptLoader) Resolve(path string) (string, error) { return path, nil }

func (p *promptLoader) Read(path string) (io.Reader, error) {
	doc, err := p.store.GetPrompt(path)
	if err != nil {
		return nil, fmt.Errorf("include %q: %w", path, err)
	}
	return strings.NewReader(doc.Prompt), nil
}

func (p *promptLoader) Inherit(string) (loaders.Loader, error) { return p, nil }
