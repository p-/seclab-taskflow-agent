// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package cli provides the cobra-based command-line interface, porting the
// Python Typer CLI: personality mode (-p), taskflow mode (-t), global
// variables (-g KEY=VALUE), model config (-m), debug (-d), and --resume.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/banner"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/runner"
)

type flags struct {
	personality string
	taskflow    string
	globals     []string
	modelConfig string
	resume      string
	debug       bool
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	var f flags
	root := &cobra.Command{
		Use:           "seclab-taskflow-agent [flags] [prompt...]",
		Short:         "SecLab Taskflow Agent (Go) — secure and automated workflow execution.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoot(cmd.Context(), &f, args)
		},
	}

	root.Flags().StringVarP(&f.personality, "personality", "p", "", "Personality module path (mutually exclusive with -t).")
	root.Flags().StringVarP(&f.taskflow, "taskflow", "t", "", "Taskflow module path (mutually exclusive with -p).")
	root.Flags().StringArrayVarP(&f.globals, "global", "g", nil, "Global variable as KEY=VALUE. Repeatable.")
	root.Flags().StringVarP(&f.modelConfig, "model-config", "m", "", "Model configuration module path.")
	root.Flags().StringVar(&f.resume, "resume", "", "Resume a previous session by its ID.")
	root.Flags().BoolVarP(&f.debug, "debug", "d", false, "Show full error details.")

	if err := root.Execute(); err != nil {
		printError(err, f.debug)
		return 1
	}
	return 0
}

func runRoot(ctx context.Context, f *flags, args []string) error {
	debug := f.debug || isTruthy(os.Getenv("TASK_AGENT_DEBUG"))
	f.debug = debug

	if f.resume != "" && (f.personality != "" || f.taskflow != "") {
		return fmt.Errorf("--resume cannot be combined with -p or -t")
	}
	if f.personality != "" && f.taskflow != "" {
		return fmt.Errorf("-p and -t are mutually exclusive")
	}
	if f.personality == "" && f.taskflow == "" && f.resume == "" {
		return fmt.Errorf("one of -p, -t, or --resume is required")
	}

	globals, err := parseGlobals(f.globals)
	if err != nil {
		return err
	}

	fmt.Println(banner.Get())

	at := loader.New()
	opts := runner.Options{
		Personality:    f.personality,
		Taskflow:       f.taskflow,
		Globals:        globals,
		Prompt:         strings.Join(args, " "),
		ResumeID:       f.resume,
		CLIModelConfig: f.modelConfig,
	}
	// When resuming, the session carries the taskflow path/globals/prompt.
	if f.resume != "" {
		opts.Taskflow = ""
	}
	return runner.RunMain(ctx, at, opts)
}

func parseGlobals(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range values {
		key, val, ok := strings.Cut(v, "=")
		if !ok {
			return nil, fmt.Errorf("invalid global variable format: %q. Expected KEY=VALUE", v)
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out, nil
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// printError prints a concise error chain unless debug is enabled, mirroring
// the Python “_print_concise_error“.
func printError(err error, debug bool) {
	if debug {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		return
	}
	seen := map[string]bool{}
	for e := err; e != nil; e = unwrap(e) {
		msg := e.Error()
		if seen[msg] {
			break
		}
		seen[msg] = true
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	}
	fmt.Fprintln(os.Stderr, "(use --debug for full details)")
}

func unwrap(err error) error {
	type wrapped interface{ Unwrap() error }
	if w, ok := err.(wrapped); ok {
		return w.Unwrap()
	}
	return nil
}
