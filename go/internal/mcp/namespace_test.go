// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompressName(t *testing.T) {
	// 12 lowercase hex characters, deterministic.
	got := CompressName("seclab_taskflow_agent.toolboxes.echo")
	require.Len(t, got, 12)
	require.Regexp(t, "^[0-9a-f]{12}$", got)
	require.Equal(t, got, CompressName("seclab_taskflow_agent.toolboxes.echo"))
	require.NotEqual(t, got, CompressName("other"))
}

func TestNamespaceRoundTrip(t *testing.T) {
	ns := CompressName("tb")
	full := addNamespace(ns, "do_thing")
	require.True(t, len(full) > len("do_thing"))
	require.Equal(t, "do_thing", stripNamespace(ns, full))
	// Idempotent re-prefixing.
	require.Equal(t, full, addNamespace(ns, full))
}
