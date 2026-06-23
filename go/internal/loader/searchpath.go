// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package loader

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SearchPathEnv is the environment variable holding extra colon-separated
// roots used to resolve dotted resource names.
const SearchPathEnv = "SECLAB_TASKFLOW_PATH"

// dottedToRelPath converts a dotted resource name such as
// “examples.taskflows.echo“ into the relative filesystem path
// “examples/taskflows/echo.yaml“. The final dotted component is the file
// stem; everything before it is the directory path.
func dottedToRelPath(dotted string) string {
	parts := strings.Split(dotted, ".")
	rel := filepath.Join(parts...)
	return rel + ".yaml"
}

// searchRoots returns the ordered list of filesystem roots in which to look
// for a dotted resource: $SECLAB_TASKFLOW_PATH entries, then the current
// working directory, then the directory containing the executable.
func searchRoots() []string {
	var roots []string
	if env := os.Getenv(SearchPathEnv); env != "" {
		for _, p := range strings.Split(env, string(os.PathListSeparator)) {
			if p != "" {
				roots = append(roots, p)
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}

// resolve locates the YAML bytes for a dotted resource name. It searches the
// filesystem roots first, then falls back to the embedded snapshot. The
// returned string is the source path (for diagnostics).
func resolve(dotted string) (data []byte, source string, err error) {
	rel := dottedToRelPath(dotted)
	for _, root := range searchRoots() {
		p := filepath.Join(root, rel)
		if b, rerr := os.ReadFile(p); rerr == nil {
			return b, p, nil
		}
	}
	// Embedded fallback uses forward slashes regardless of OS.
	embedded := "embedded/" + filepath.ToSlash(rel)
	if b, rerr := fs.ReadFile(bundledFS, embedded); rerr == nil {
		return b, "embedded:" + filepath.ToSlash(rel), nil
	}
	return nil, "", &NotFoundError{Dotted: dotted, Rel: rel}
}

// NotFoundError indicates a dotted resource could not be located in any root.
type NotFoundError struct {
	Dotted string
	Rel    string
}

func (e *NotFoundError) Error() string {
	return "cannot load " + e.Dotted + ": no file " + e.Rel + " on search path or in embedded resources"
}
