// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/grammar"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/loader"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/session"
)

func setupRunnerAsyncFixture(t *testing.T) *loader.AvailableTools {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "fixtures", "personality.yaml"), `
seclab-taskflow-agent:
  version: "1.0"
  filetype: personality
personality: You are a test agent.
task: Return success.
`)
	t.Setenv(loader.SearchPathEnv, root)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOG_DIR", t.TempDir())
	return loader.New()
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func newTestSession() *session.Session {
	return session.New("fixtures.flow", map[string]string{}, "", 1, "")
}

func TestRunTaskAsyncRepeatPromptHonorsAsyncLimit(t *testing.T) {
	at := setupRunnerAsyncFixture(t)
	orig := deployTaskAgentsFunc
	defer func() { deployTaskAgentsFunc = orig }()

	var active int32
	var maxActive int32
	var calls int32
	deployTaskAgentsFunc = func(context.Context, *loader.AvailableTools, deployParams) (bool, error) {
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			max := atomic.LoadInt32(&maxActive)
			if current <= max || atomic.CompareAndSwapInt32(&maxActive, max, current) {
				break
			}
		}
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return true, nil
	}

	lastResults := []string{`{"text":"[1,2,3,4]"}`}
	task := grammar.TaskDefinition{
		Name:         "async-repeat",
		Agents:       []string{"fixtures.personality"},
		UserPrompt:   "value {{ result }}",
		RepeatPrompt: true,
		AsyncTask:    true,
		AsyncLimit:   2,
	}
	err := runTask(context.Background(), at, &grammar.TaskflowDocument{}, nil, 0, task, nil, "", newTestSession(), &lastResults, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int32(4), atomic.LoadInt32(&calls))
	require.LessOrEqual(t, atomic.LoadInt32(&maxActive), int32(2))
	require.Greater(t, atomic.LoadInt32(&maxActive), int32(1))
}

func TestRunTaskAsyncRepeatPromptAggregatesFailuresAfterAllSubtasks(t *testing.T) {
	at := setupRunnerAsyncFixture(t)
	orig := deployTaskAgentsFunc
	defer func() { deployTaskAgentsFunc = orig }()

	var mu sync.Mutex
	var prompts []string
	var sawSyncDeploy int32
	deployTaskAgentsFunc = func(_ context.Context, _ *loader.AvailableTools, dp deployParams) (bool, error) {
		if !dp.asyncTask {
			atomic.StoreInt32(&sawSyncDeploy, 1)
		}
		mu.Lock()
		prompts = append(prompts, dp.prompt)
		mu.Unlock()
		return !strings.Contains(dp.prompt, "2"), nil
	}

	lastResults := []string{`{"text":"[1,2,3]"}`}
	sess := newTestSession()
	task := grammar.TaskDefinition{
		Name:         "async-repeat",
		Agents:       []string{"fixtures.personality"},
		UserPrompt:   "value {{ result }}",
		RepeatPrompt: true,
		AsyncTask:    true,
		AsyncLimit:   3,
	}
	err := runTask(context.Background(), at, &grammar.TaskflowDocument{}, nil, 0, task, nil, "", sess, &lastResults, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int32(0), atomic.LoadInt32(&sawSyncDeploy))
	require.Len(t, prompts, 3)
	require.Len(t, sess.CompletedTasks, 1)
	require.False(t, sess.CompletedTasks[0].Result)
}

func TestRunTaskAsyncWithoutRepeatPromptRunsSequentially(t *testing.T) {
	at := setupRunnerAsyncFixture(t)
	orig := deployTaskAgentsFunc
	defer func() { deployTaskAgentsFunc = orig }()

	var calls int32
	var sawAsyncDeploy bool
	deployTaskAgentsFunc = func(_ context.Context, _ *loader.AvailableTools, dp deployParams) (bool, error) {
		atomic.AddInt32(&calls, 1)
		sawAsyncDeploy = dp.asyncTask
		return true, nil
	}

	lastResults := []string{}
	sess := newTestSession()
	task := grammar.TaskDefinition{
		Name:       "async-no-repeat",
		Agents:     []string{"fixtures.personality"},
		UserPrompt: "hello",
		AsyncTask:  true,
		AsyncLimit: 2,
	}
	err := runTask(context.Background(), at, &grammar.TaskflowDocument{}, nil, 0, task, nil, "", sess, &lastResults, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	require.False(t, sawAsyncDeploy)
	require.Len(t, sess.CompletedTasks, 1)
	require.True(t, sess.CompletedTasks[0].Result)
}

func TestRunTaskPassesExcludeFromContextToDeploy(t *testing.T) {
	at := setupRunnerAsyncFixture(t)
	orig := deployTaskAgentsFunc
	defer func() { deployTaskAgentsFunc = orig }()

	var sawExclude bool
	deployTaskAgentsFunc = func(_ context.Context, _ *loader.AvailableTools, dp deployParams) (bool, error) {
		sawExclude = dp.excludeCtx
		return true, nil
	}

	lastResults := []string{}
	task := grammar.TaskDefinition{
		Name:               "exclude-context",
		Agents:             []string{"fixtures.personality"},
		UserPrompt:         "hello",
		ExcludeFromContext: true,
	}
	err := runTask(context.Background(), at, &grammar.TaskflowDocument{}, nil, 0, task, nil, "", newTestSession(), &lastResults, nil, nil)
	require.NoError(t, err)
	require.True(t, sawExclude)
}
