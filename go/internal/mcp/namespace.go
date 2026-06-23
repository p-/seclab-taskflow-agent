// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package mcp resolves toolbox YAML into MCP connection parameters, connects
// to MCP servers via the modelcontextprotocol/go-sdk, and presents their
// tools to a backend with namespace-prefixed names. It ports the relevant
// parts of the Python “mcp_utils“/“mcp_lifecycle“/“mcp_transport“
// modules.
package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// compressedNameLength is the number of hex characters used to namespace
// tool names. The OpenAI API rejects tool names longer than 64 characters,
// so long toolbox names are hashed to a short prefix.
const compressedNameLength = 12

// CompressName returns a short SHA-256 hash of name, matching the Python
// “compress_name“ helper (first 12 hex characters of the digest).
func CompressName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])[:compressedNameLength]
}

// addNamespace prefixes a tool name with the namespace, idempotently (an
// already-prefixed name is not double-prefixed).
func addNamespace(namespace, name string) string {
	return namespace + strings.TrimPrefix(name, namespace)
}

// stripNamespace removes the namespace prefix from a tool name.
func stripNamespace(namespace, name string) string {
	return strings.TrimPrefix(name, namespace)
}
