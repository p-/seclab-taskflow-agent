// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/envutil"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/render"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/session"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/shell"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/tmpl"
)

// DefaultMaxTurns bounds agent turns before forced termination.
const DefaultMaxTurns = 50

// taskRetryLimit and related constants mirror the Python runner.
const (
	taskRetryLimit = 3
)

// Options configures a RunMain invocation.
type Options struct {
	Personality    string
	Taskflow       string
	Globals        map[string]string
	Prompt         string
	ResumeID       string
	CLIModelConfig string
}

// RunMain is the top-level entry point for personality/taskflow execution.
func RunMain(ctx context.Context, at *loader.AvailableTools, opts Options) error {
	lastResults := []string{}
	var lastResultsMu sync.Mutex

	onToolStart := func(name string, asyncTask bool, taskID string) {
		render.OutputMaybeBufferedf(asyncTask, taskID, "\n** \U0001F916\U0001F6E0\uFE0F Tool Call: %s\n", name)
	}
	onTool := func(_ string, result string) {
		payload, _ := json.Marshal(map[string]string{"text": result})
		lastResultsMu.Lock()
		defer lastResultsMu.Unlock()
		lastResults = append(lastResults, string(payload))
	}

	if opts.Personality != "" {
		personality, err := at.GetPersonality(opts.Personality)
		if err != nil {
			return err
		}
		model, err := resolveTaskModel(grammar.TaskDefinition{}, nil)
		if err != nil {
			return err
		}
		_, err = deployTaskAgentsFunc(ctx, at, deployParams{
			agents:      map[string]*grammar.PersonalityDocument{opts.Personality: personality},
			agentOrder:  []string{opts.Personality},
			prompt:      opts.Prompt,
			maxTurns:    DefaultMaxTurns,
			model:       model,
			onTool:      onTool,
			onToolStart: onToolStart,
		})
		return err
	}

	if opts.Taskflow == "" && opts.ResumeID == "" {
		return fmt.Errorf("one of personality, taskflow, or resume is required")
	}

	return runTaskflow(ctx, at, opts, &lastResults, onTool, onToolStart)
}

func runTaskflow(
	ctx context.Context,
	at *loader.AvailableTools,
	opts Options,
	lastResults *[]string,
	onTool func(string, string),
	onToolStart func(string, bool, string),
) error {
	taskflowPath := opts.Taskflow
	cliGlobals := opts.Globals
	prompt := opts.Prompt
	cliModelConfig := opts.CLIModelConfig

	var sess *session.Session
	if opts.ResumeID != "" {
		s, err := session.Load(opts.ResumeID)
		if err != nil {
			return err
		}
		if s.Finished {
			render.Outputf("** \U0001F916\u2705 Session %s already completed\n", opts.ResumeID)
			return nil
		}
		sess = s
		taskflowPath = s.TaskflowPath
		cliGlobals = s.CLIGlobals
		prompt = s.Prompt
		*lastResults = append([]string(nil), s.LastToolResults...)
		if cliModelConfig == "" && s.CLIModelConfig != "" {
			cliModelConfig = s.CLIModelConfig
		}
		render.Outputf("** \U0001F916\U0001F504 Resuming session %s from task %d\n", opts.ResumeID, s.NextTaskIndex())
	}

	doc, err := at.GetTaskflow(taskflowPath)
	if err != nil {
		return err
	}
	render.Outputf("** \U0001F916\U0001F4AA Running Task Flow: %s\n", taskflowPath)

	globals := map[string]any{}
	for k, v := range doc.Globals {
		globals[k] = v
	}
	for k, v := range cliGlobals {
		globals[k] = v
	}

	modelConfigRef := doc.ModelConfigRef
	if cliModelConfig != "" {
		modelConfigRef = cliModelConfig
	}
	var mc *modelConfig
	if modelConfigRef != "" {
		mc, err = resolveModelConfig(at, modelConfigRef)
		if err != nil {
			return err
		}
	}

	if sess == nil {
		sess = session.New(taskflowPath, cliGlobals, prompt, len(doc.Taskflow), opts.CLIModelConfig)
		if _, err := sess.Save(); err != nil {
			return err
		}
		render.Outputf("** \U0001F916\U0001F4CB Session: %s\n", sess.SessionID)
	}

	for idx, wrapper := range doc.Taskflow {
		if idx < sess.NextTaskIndex() {
			render.Outputf("** \U0001F916\u23ED\uFE0F Skipping completed task %d\n", idx)
			continue
		}
		if err := runTask(ctx, at, doc, mc, idx, wrapper.Task, globals, prompt, sess, lastResults, onTool, onToolStart); err != nil {
			return err
		}
		if sess.Error != "" {
			return nil // must_complete failure already recorded
		}
	}

	if sess.Error == "" {
		if err := sess.MarkFinished(); err != nil {
			return err
		}
		render.Outputf("** \U0001F916\u2705 Session %s completed\n", sess.SessionID)
	}
	return nil
}

// runTask executes a single task with templating, env scoping, and retry.
func runTask(
	ctx context.Context,
	at *loader.AvailableTools,
	doc *grammar.TaskflowDocument,
	mc *modelConfig,
	idx int,
	task grammar.TaskDefinition,
	globals map[string]any,
	cliPrompt string,
	sess *session.Session,
	lastResults *[]string,
	onTool func(string, string),
	onToolStart func(string, bool, string),
) error {
	if task.Uses != "" {
		merged, err := mergeReusableTask(at, task)
		if err != nil {
			return err
		}
		task = merged
	}

	model, err := resolveTaskModel(task, mc)
	if err != nil {
		return err
	}

	taskPrompt := task.UserPrompt
	if taskPrompt != "" && !task.RepeatPrompt {
		rendered, err := tmpl.Render(at, taskPrompt, tmpl.RenderContext{Globals: globals, Inputs: task.Inputs})
		if err != nil {
			return fmt.Errorf("failed to render prompt template: %w", err)
		}
		taskPrompt = rendered
	}

	// Apply task-scoped environment variables.
	tmpEnv, err := envutil.Apply(task.Env, map[string]any{"globals": globals})
	if err != nil {
		return err
	}
	defer tmpEnv.Restore()

	prompts, err := buildPromptsToRun(at, taskPrompt, task.RepeatPrompt, lastResults, globals, task.Inputs)
	if err != nil {
		return err
	}

	maxTurns := task.MaxSteps
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	}

	run := func() (bool, error) {
		if task.Run != "" {
			render.Output("** \U0001F916\U0001F41A Executing Shell Task\n")
			result, err := shell.RunResultJSON(task.Run)
			if err != nil {
				render.Outputf("** \U0001F916\u2757 Shell Task Exception: %s\n", err.Error())
				return false, nil
			}
			*lastResults = append(*lastResults, result)
			return true, nil
		}

		type deployment struct {
			prompt string
			agents map[string]*grammar.PersonalityDocument
			order  []string
		}

		deployments := make([]deployment, 0, len(prompts))
		for _, p := range prompts {
			agents, order, err := resolveAgents(at, task, p)
			if err != nil {
				return false, err
			}
			deployments = append(deployments, deployment{prompt: p, agents: agents, order: order})
		}

		if !task.AsyncTask || !task.RepeatPrompt {
			complete := true
			for _, d := range deployments {
				ok, derr := deployTaskAgentsFunc(ctx, at, deployParams{
					agents:      d.agents,
					agentOrder:  d.order,
					prompt:      d.prompt,
					toolboxes:   task.Toolboxes,
					blockedTool: task.BlockedTools,
					headless:    task.Headless,
					excludeCtx:  task.ExcludeFromContext,
					maxTurns:    maxTurns,
					model:       model,
					onTool:      onTool,
					onToolStart: onToolStart,
				})
				if derr != nil {
					return false, derr
				}
				complete = complete && ok
			}
			return complete, nil
		}

		if task.AsyncLimit <= 0 {
			return false, fmt.Errorf("task %q has invalid async_limit %d", taskName(task, idx), task.AsyncLimit)
		}

		sem := make(chan struct{}, task.AsyncLimit)
		results := make([]bool, len(deployments))
		errs := make([]error, len(deployments))
		var wg sync.WaitGroup
		for i, d := range deployments {
			wg.Add(1)
			go func(i int, d deployment) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					errs[i] = ctx.Err()
					return
				}

				ok, derr := deployTaskAgentsFunc(ctx, at, deployParams{
					agents:      d.agents,
					agentOrder:  d.order,
					prompt:      d.prompt,
					toolboxes:   task.Toolboxes,
					blockedTool: task.BlockedTools,
					headless:    task.Headless,
					excludeCtx:  task.ExcludeFromContext,
					maxTurns:    maxTurns,
					model:       model,
					onTool:      onTool,
					onToolStart: onToolStart,
					asyncTask:   true,
				})
				results[i] = ok
				errs[i] = derr
			}(i, d)
		}
		wg.Wait()

		complete := true
		for i, ok := range results {
			if errs[i] != nil {
				return false, errs[i]
			}
			complete = complete && ok
		}
		return complete, nil
	}

	name := taskName(task, idx)
	var complete bool
	var lastErr error
	for attempt := 0; attempt < taskRetryLimit; attempt++ {
		complete, lastErr = run()
		if lastErr == nil {
			break
		}
		render.Outputf("** \U0001F916\U0001F504 Task %q failed: %s\n", name, lastErr.Error())
	}

	if lastErr != nil {
		_ = sess.MarkFailed(fmt.Sprintf("Task %q: %s", name, lastErr.Error()))
		render.Outputf("** \U0001F916\U0001F4BE Session saved: %s\n** \U0001F916\U0001F4A1 Resume with: --resume %s\n", sess.SessionID, sess.SessionID)
		return lastErr
	}

	if task.MustComplete && !complete {
		render.Output("\U0001F916\U0001F4A5 *Required task not completed ...\n")
		_ = sess.MarkFailed(fmt.Sprintf("Required task %q did not complete", name))
		render.Outputf("** \U0001F916\U0001F4BE Session saved: %s\n** \U0001F916\U0001F4A1 Resume with: --resume %s\n", sess.SessionID, sess.SessionID)
		return nil
	}

	return sess.RecordTask(idx, name, complete, append([]string(nil), *lastResults...))
}

// resolveAgents resolves the personalities for a task. When no agents are
// listed the prompt itself is parsed for a leading “-p name“ flag.
func resolveAgents(at *loader.AvailableTools, task grammar.TaskDefinition, p string) (map[string]*grammar.PersonalityDocument, []string, error) {
	names := append([]string(nil), task.Agents...)
	if len(names) == 0 {
		if pname := leadingPersonality(p); pname != "" {
			names = append(names, pname)
		}
	}
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("no agents resolved for this task; specify a personality with -p or an agents list")
	}
	agents := map[string]*grammar.PersonalityDocument{}
	for _, n := range names {
		pd, err := at.GetPersonality(n)
		if err != nil {
			return nil, nil, err
		}
		agents[n] = pd
	}
	return agents, names, nil
}

func taskName(task grammar.TaskDefinition, idx int) string {
	if task.Name != "" {
		return task.Name
	}
	return fmt.Sprintf("task-%d", idx)
}
