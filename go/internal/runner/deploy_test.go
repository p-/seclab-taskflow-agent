// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatModelLine(t *testing.T) {
	require.Equal(t, "gpt-5-mini", formatModelLine("gpt-5-mini", nil))
	require.Equal(t, "gpt-5-mini", formatModelLine("gpt-5-mini", map[string]any{}))

	got := formatModelLine("gpt-5-mini", map[string]any{"temperature": 1})
	require.Equal(t, "gpt-5-mini, params: map[temperature:1]", got)

	// Multiple settings are rendered with deterministic key ordering by fmt.
	got = formatModelLine("gpt-5-mini", map[string]any{
		"temperature": 1,
		"reasoning":   map[string]any{"effort": "high"},
	})
	require.Equal(t, "gpt-5-mini, params: map[reasoning:map[effort:high] temperature:1]", got)
}
