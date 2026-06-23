// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package grammar defines the YAML grammar for taskflows, personalities,
// toolboxes, model configs, and prompts. It ports the Pydantic models from
// the Python “models.py“ module, preserving field names, aliases, defaults,
// and validation so existing YAML files parse unchanged.
package grammar

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// SupportedVersion is the only grammar version the agent accepts.
const SupportedVersion = "1.0"

// Header is the “seclab-taskflow-agent“ block present in every YAML file.
type Header struct {
	Version  string `yaml:"version"`
	Filetype string `yaml:"filetype"`
}

// UnmarshalYAML normalises int/float/string versions to the “"1.0"“ form
// and validates the version, matching the Python field validators.
func (h *Header) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Version  yaml.Node `yaml:"version"`
		Filetype string    `yaml:"filetype"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	h.Version = normaliseVersion(raw.Version)
	h.Filetype = raw.Filetype
	if h.Version != SupportedVersion {
		return fmt.Errorf("unsupported version: %s. Only version %s is supported", h.Version, SupportedVersion)
	}
	return nil
}

// normaliseVersion accepts int (“1“ -> “"1.0"“), float (“1.0“ ->
// “"1.0"“), or string versions and returns the string form.
func normaliseVersion(n yaml.Node) string {
	s := n.Value
	if n.Tag == "!!int" {
		if i, err := strconv.Atoi(s); err == nil {
			return fmt.Sprintf("%d.0", i)
		}
	}
	if n.Tag == "!!float" {
		// The raw YAML text (e.g. "1.0") already carries the decimal form
		// that Python's str(float) would produce.
		return s
	}
	return s
}
