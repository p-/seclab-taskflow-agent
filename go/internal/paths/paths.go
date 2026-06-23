// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package paths provides platform-aware data and log directory resolution,
// mirroring the Python “path_utils“ module. Directories follow the XDG
// base-directory specification via the adrg/xdg library.
package paths

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const (
	appName   = "seclab-taskflow-agent"
	appAuthor = "GitHubSecurityLab"
)

// DataDir returns (creating if necessary) the top-level application data
// directory used for sessions and MCP server state.
func DataDir() (string, error) {
	d := filepath.Join(xdg.DataHome, appAuthor, appName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// SessionDir returns (creating if necessary) the directory used for session
// checkpoint files.
func SessionDir() (string, error) {
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "sessions")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// LogDir returns (creating if necessary) the directory used for log files.
// The “LOG_DIR“ environment variable overrides the default location.
func LogDir() (string, error) {
	d := os.Getenv("LOG_DIR")
	if d == "" {
		d = filepath.Join(xdg.StateHome, appAuthor, appName, "logs")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// LogFile returns the full path to a named log file in the log directory.
func LogFile(name string) (string, error) {
	d, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}

// MCPDataDir returns (creating if necessary) a per-server data directory.
// When envOverride names a set environment variable, its value is used
// verbatim instead of the default location.
func MCPDataDir(packageName, mcpName, envOverride string) (string, error) {
	if envOverride != "" {
		if p := os.Getenv(envOverride); p != "" {
			return p, nil
		}
	}
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, packageName, mcpName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}
