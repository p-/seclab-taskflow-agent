// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/mcp"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/prompt"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/render"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/stream"

	_ "github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk/openai" // register the openai backend
)

// importantGuidelines are appended to every system prompt, matching Python.
var importantGuidelines = []string{
	"Do not prompt the user with questions.",
	"Run tasks until a final result is available.",
	"Ensure responses are based on the latest information from available tools.",
	"Run tools sequentially, wait until one tool has completed before calling the next.",
}

// deployParams bundles the inputs for deploying a single task agent.
type deployParams struct {
	agents      map[string]*grammar.PersonalityDocument
	agentOrder  []string
	prompt      string
	toolboxes   []string
	blockedTool []string
	headless    bool
	excludeCtx  bool
	maxTurns    int
	model       resolvedTaskModel
	onTool      stream.ToolHook
	onToolStart func(name string, asyncTask bool, taskID string)
	asyncTask   bool
}

var deployTaskAgentsFunc = deployTaskAgents

// deployTaskAgents connects MCP servers, builds the backend agent, and runs
// the prompt to completion. It ports the Python “deploy_task_agents“ for the
// single-agent MVP (handoffs are rejected by the backend's Validate).
func deployTaskAgents(ctx context.Context, at *loader.AvailableTools, dp deployParams) (bool, error) {
	taskID := newDeployTaskID()
	if dp.asyncTask {
		defer render.FlushAsyncOutput(taskID)
	}
	render.OutputMaybeBufferedf(dp.asyncTask, taskID, "** \U0001F916\U0001F4AA Deploying Task Flow Agent(s): %v\n", dp.agentOrder)
	render.OutputMaybeBufferedf(dp.asyncTask, taskID, "** \U0001F916\U0001F4AA Task ID : %s\n", taskID)
	render.OutputMaybeBufferedf(dp.asyncTask, taskID, "** \U0001F916\U0001F4AA Model   : %s\n", dp.model.model)
	if dp.model.endpoint != "" {
		render.OutputMaybeBufferedf(dp.asyncTask, taskID, "** \U0001F916\U0001F4AA Endpoint: %s\n", dp.model.endpoint)
	}

	// Resolve toolboxes: explicit override or union from personalities.
	toolboxes := dp.toolboxes
	if len(toolboxes) == 0 {
		seen := map[string]bool{}
		for _, name := range dp.agentOrder {
			for _, tb := range dp.agents[name].Toolboxes {
				if !seen[tb] {
					seen[tb] = true
					toolboxes = append(toolboxes, tb)
				}
			}
		}
	}

	params, err := mcp.ResolveParams(at, toolboxes)
	if err != nil {
		return false, err
	}

	pool, err := mcp.Connect(ctx, params)
	if err != nil {
		return false, err
	}
	pool.SetHeadless(dp.headless)
	defer pool.Close(ctx)

	serverPrompts := pool.ServerPrompts(params)

	// Build the primary agent spec (single-personality MVP).
	primaryName := dp.agentOrder[0]
	primary := dp.agents[primaryName]
	spec := &sdk.AgentSpec{
		Name:               primaryName,
		Instructions:       prompt.BuildSystemPrompt(primary.Personality, primary.Task, importantGuidelines, serverPrompts),
		Model:              dp.model.model,
		ModelSettings:      dp.model.settings,
		MCPServers:         pool.Specs(),
		ExcludeFromContext: dp.excludeCtx,
		APIType:            dp.model.apiType,
		Endpoint:           dp.model.endpoint,
		TokenEnv:           dp.model.token,
		BlockedTools:       dp.blockedTool,
		Headless:           dp.headless,
	}

	backendName := sdk.ResolveName(dp.model.backend)
	backend, err := sdk.Get(backendName)
	if err != nil {
		return false, err
	}
	if err := backend.Validate(spec); err != nil {
		return false, err
	}

	agent, err := backend.Build(ctx, spec)
	if err != nil {
		return false, err
	}
	defer agent.Close(ctx)

	if err := stream.Drive(ctx, backend, agent, dp.prompt, dp.maxTurns, dp.onToolStart, dp.onTool, dp.asyncTask, taskID); err != nil {
		render.OutputMaybeBufferedf(dp.asyncTask, taskID, "** \U0001F916\u2757 %s\n", err.Error())
		return false, err
	}
	return true, nil
}

func newDeployTaskID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}
