// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package envutil provides environment-variable helpers: a temporary
// environment scope (ports “env_utils.TmpEnv“) and a denylist filter for
// MCP subprocess environments (ports “mcp_transport._filtered_env“).
package envutil

import (
	"os"
	"strings"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/tmpl"
)

// DenylistEnv names the variable holding a comma-separated list of variables
// that must not be forwarded to MCP server subprocesses.
const DenylistEnv = "TASKFLOW_ENV_DENYLIST"

// TmpEnv temporarily sets environment variables, rendering each value through
// the template engine (so “{{ env(...) }}“ and supplied context resolve),
// and restores the previous environment when Restore is called.
type TmpEnv struct {
	applied map[string]*string // var -> previous value (nil if it was unset)
}

// Apply sets the given variables, expanding their values via tmpl.SwapEnv with
// the supplied context. On error, any partial changes are rolled back.
func Apply(env map[string]string, context map[string]any) (*TmpEnv, error) {
	t := &TmpEnv{applied: map[string]*string{}}
	for k, raw := range env {
		v, err := tmpl.SwapEnv(raw, context)
		if err != nil {
			t.Restore()
			return nil, err
		}
		if prev, ok := os.LookupEnv(k); ok {
			p := prev
			t.applied[k] = &p
		} else {
			t.applied[k] = nil
		}
		if err := os.Setenv(k, v); err != nil {
			t.Restore()
			return nil, err
		}
	}
	return t, nil
}

// Restore reverts the environment to its state before Apply.
func (t *TmpEnv) Restore() {
	for k, prev := range t.applied {
		if prev == nil {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, *prev)
		}
	}
	t.applied = map[string]*string{}
}

// FilteredEnviron returns a copy of the process environment as a slice of
// “KEY=VALUE“ strings with denylisted variables removed.
func FilteredEnviron() []string {
	denied := deniedSet()
	if len(denied) == 0 {
		return os.Environ()
	}
	var out []string
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if !denied[key] {
			out = append(out, kv)
		}
	}
	return out
}

func deniedSet() map[string]bool {
	raw := os.Getenv(DenylistEnv)
	if raw == "" {
		return nil
	}
	set := map[string]bool{}
	for _, k := range strings.Split(raw, ",") {
		if k = strings.TrimSpace(k); k != "" {
			set[k] = true
		}
	}
	return set
}
