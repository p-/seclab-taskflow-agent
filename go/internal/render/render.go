// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package render writes streamed model output to stdout, mirroring it to a
// log file. It ports the synchronous parts of the Python “render_utils“
// module (async output buffering is deferred along with async tasks).
package render

import (
	"fmt"
	"os"
	"sync"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/paths"
)

var (
	mu      sync.Mutex
	logFile *os.File
	logOnce sync.Once
)

func openLog() {
	p, err := paths.LogFile("render_stdout.log")
	if err != nil {
		return
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		logFile = f
	}
}

// Output prints data to stdout and appends it to the render log. It is safe
// for concurrent use.
func Output(data string) {
	mu.Lock()
	defer mu.Unlock()
	logOnce.Do(openLog)
	if logFile != nil {
		_, _ = logFile.WriteString(data)
	}
	fmt.Print(data)
}

// Outputf is a convenience wrapper around Output with formatting.
func Outputf(format string, args ...any) {
	Output(fmt.Sprintf(format, args...))
}
