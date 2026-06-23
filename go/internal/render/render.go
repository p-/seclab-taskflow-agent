// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package render writes streamed model output to stdout, mirroring it to a
// log file. It ports the Python “render_utils“ module, including buffering
// streamed output from async repeat_prompt fan-out.
package render

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/paths"
)

var (
	mu           sync.Mutex
	logFile      *os.File
	logOnce      sync.Once
	asyncBuffers = map[string]*strings.Builder{}
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
	writeLocked(data)
}

func writeLocked(data string) {
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

// OutputMaybeBuffered writes data immediately for normal tasks and buffers it
// for async tasks so concurrent fan-out output stays readable.
func OutputMaybeBuffered(data string, asyncTask bool, taskID string) {
	if !asyncTask || taskID == "" {
		Output(data)
		return
	}

	mu.Lock()
	defer mu.Unlock()
	buf, ok := asyncBuffers[taskID]
	if !ok {
		buf = &strings.Builder{}
		asyncBuffers[taskID] = buf
		buf.WriteString(data)
		writeLocked("** \U0001F916\u270F\uFE0F Gathering output from async task ... please hold\n")
		return
	}
	buf.WriteString(data)
}

// OutputMaybeBufferedf is a convenience wrapper around OutputMaybeBuffered.
func OutputMaybeBufferedf(asyncTask bool, taskID string, format string, args ...any) {
	OutputMaybeBuffered(fmt.Sprintf(format, args...), asyncTask, taskID)
}

// FlushAsyncOutput emits and clears buffered output for one async task. It is
// a no-op if the task produced no buffered output.
func FlushAsyncOutput(taskID string) {
	if taskID == "" {
		return
	}

	mu.Lock()
	buf, ok := asyncBuffers[taskID]
	if !ok {
		mu.Unlock()
		return
	}
	data := buf.String()
	delete(asyncBuffers, taskID)
	mu.Unlock()

	Outputf("** \U0001F916\u270F\uFE0F Output for async task: %s\n\n", taskID)
	Output(data)
}
