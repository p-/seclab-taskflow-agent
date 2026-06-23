// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package tmpl

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
)

// RenderContext carries the variables exposed to a template render. Each
// field maps to a top-level Jinja namespace: “globals“, “inputs“ and the
// per-iteration “result“ used by repeat-prompt tasks.
type RenderContext struct {
	Globals map[string]any
	Inputs  map[string]any
	// Result is included as the ``result`` variable only when non-nil.
	Result any
}

// Render renders templateStr against the given context. Reusable prompts
// referenced via “{% include 'dotted.path' %}“ are resolved through store.
func Render(store PromptStore, templateStr string, rc RenderContext) (string, error) {
	cfg := jinjaConfig()
	env := environment()

	sub := &promptLoader{store: store}
	rootID := fmt.Sprintf("root-%x", sha256.Sum256([]byte(templateStr)))
	shifted, err := loaders.NewShiftedLoader(rootID, bytes.NewReader([]byte(templateStr)), sub)
	if err != nil {
		return "", err
	}

	tpl, err := exec.NewTemplate(rootID, cfg, shifted, env)
	if err != nil {
		return "", err
	}

	data := map[string]any{
		"globals": orEmpty(rc.Globals),
		"inputs":  orEmpty(rc.Inputs),
	}
	if rc.Result != nil {
		data["result"] = rc.Result
	}

	out, err := tpl.ExecuteToString(exec.NewContext(data))
	if err != nil {
		return "", err
	}
	return out, nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// SwapEnv renders a standalone string that may contain “{{ env('VAR') }}“
// expressions and the supplied context variables. It ports
// “env_utils.swap_env“ and is used to expand toolbox env/args/headers.
func SwapEnv(s string, context map[string]any) (string, error) {
	cfg := jinjaConfig()
	env := environment()
	rootID := fmt.Sprintf("root-%x", sha256.Sum256([]byte(s)))
	// SwapEnv has no include support; an empty memory loader suffices.
	sub := loaders.MustNewMemoryLoader(map[string]string{})
	shifted, err := loaders.NewShiftedLoader(rootID, bytes.NewReader([]byte(s)), sub)
	if err != nil {
		return "", err
	}
	tpl, err := exec.NewTemplate(rootID, cfg, shifted, env)
	if err != nil {
		return "", err
	}
	// Filter out keys colliding with built-in globals (e.g. env helper).
	data := map[string]any{}
	for k, v := range context {
		if !env.Context.Has(k) {
			data[k] = v
		}
	}
	return tpl.ExecuteToString(exec.NewContext(data))
}
