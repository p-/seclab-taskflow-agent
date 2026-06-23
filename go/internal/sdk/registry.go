// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package sdk

import (
	"fmt"
	"os"
	"sort"
	"sync"
)

// BackendEnv is the environment variable that selects a backend by name.
const BackendEnv = "SECLAB_TASKFLOW_BACKEND"

// DefaultBackend is used when no explicit or env-configured backend is given.
const DefaultBackend = "openai"

var (
	registryMu sync.RWMutex
	registry   = map[string]Backend{}
)

// Register adds a backend to the registry. Adapters call this from init().
func Register(b Backend) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[b.Name()] = b
}

// Get returns the registered backend for name.
func Get(name string) (Backend, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q. Known: %v", name, knownNames())
	}
	return b, nil
}

// knownNames returns the sorted set of registered backend names.
func knownNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ResolveName picks the backend name for a run. Precedence: explicit (from
// model config) > SECLAB_TASKFLOW_BACKEND > DefaultBackend.
func ResolveName(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(BackendEnv); env != "" {
		return env
	}
	return DefaultBackend
}
