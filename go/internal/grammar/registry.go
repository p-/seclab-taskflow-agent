// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package grammar

// Filetype values for the “filetype“ header field.
const (
	FiletypePersonality = "personality"
	FiletypeTaskflow    = "taskflow"
	FiletypePrompt      = "prompt"
	FiletypeToolbox     = "toolbox"
	FiletypeModelConfig = "model_config"
)

// KnownFiletypes is the set of recognised document filetypes.
var KnownFiletypes = map[string]bool{
	FiletypePersonality: true,
	FiletypeTaskflow:    true,
	FiletypePrompt:      true,
	FiletypeToolbox:     true,
	FiletypeModelConfig: true,
}
