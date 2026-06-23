// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package session persists taskflow progress for checkpoint/resume. It ports
// the Python “session“ module: state is saved as JSON in the application
// data directory after each completed task.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/paths"
)

// CompletedTask records a single completed task within a session.
type CompletedTask struct {
	Index       int      `json:"index"`
	Name        string   `json:"name"`
	Result      bool     `json:"result"`
	ToolResults []string `json:"tool_results"`
}

// Session is the persistent state for a taskflow run.
type Session struct {
	SessionID       string            `json:"session_id"`
	TaskflowPath    string            `json:"taskflow_path"`
	CLIGlobals      map[string]string `json:"cli_globals"`
	Prompt          string            `json:"prompt"`
	CreatedAt       string            `json:"created_at"`
	UpdatedAt       string            `json:"updated_at"`
	CompletedTasks  []CompletedTask   `json:"completed_tasks"`
	TotalTasks      int               `json:"total_tasks"`
	Finished        bool              `json:"finished"`
	Error           string            `json:"error"`
	CLIModelConfig  string            `json:"cli_model_config"`
	LastToolResults []string          `json:"last_tool_results"`
}

// New creates a fresh session with a random 12-hex-character ID.
func New(taskflowPath string, cliGlobals map[string]string, prompt string, totalTasks int, cliModelConfig string) *Session {
	return &Session{
		SessionID:      newID(),
		TaskflowPath:   taskflowPath,
		CLIGlobals:     cliGlobals,
		Prompt:         prompt,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		TotalTasks:     totalTasks,
		CLIModelConfig: cliModelConfig,
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NextTaskIndex returns the index of the next task to execute.
func (s *Session) NextTaskIndex() int {
	maxIdx := -1
	for _, t := range s.CompletedTasks {
		if t.Index > maxIdx {
			maxIdx = t.Index
		}
	}
	return maxIdx + 1
}

func filePath(id string) (string, error) {
	dir, err := paths.SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// Save persists the session to disk and returns the file path.
func (s *Session) Save() (string, error) {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	p, err := filePath(s.SessionID)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// RecordTask appends a completed task and saves the checkpoint.
func (s *Session) RecordTask(index int, name string, success bool, toolResults []string) error {
	s.CompletedTasks = append(s.CompletedTasks, CompletedTask{
		Index:       index,
		Name:        name,
		Result:      success,
		ToolResults: toolResults,
	})
	s.LastToolResults = append([]string(nil), toolResults...)
	_, err := s.Save()
	return err
}

// MarkFinished marks the session complete and saves.
func (s *Session) MarkFinished() error {
	s.Finished = true
	_, err := s.Save()
	return err
}

// MarkFailed records an error message and saves.
func (s *Session) MarkFailed(msg string) error {
	s.Error = msg
	_, err := s.Save()
	return err
}

// Load reads a session from disk by ID.
func Load(id string) (*Session, error) {
	p, err := filePath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("no session checkpoint found: %s", id)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
