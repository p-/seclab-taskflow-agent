// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

// Package stream drives a backend's event stream to completion with idle
// timeouts, rate-limit backoff, and bounded retries. It ports the Python
// “_stream“ module's “drive_backend_stream“.
package stream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/render"
	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// Defaults mirror the Python runner constants.
const (
	IdleTimeout         = 30 * time.Minute
	MaxAPIRetry         = 5
	InitialRateBackoff  = 5 * time.Second
	MaxRateLimitBackoff = 120 * time.Second
)

// ToolHook is invoked for each completed tool call so the runner can capture
// MCP results for repeat_prompt and checkpointing.
type ToolHook func(toolName, result string)

// Drive runs the backend stream to completion. TextDelta events are rendered
// to stdout; ToolEnd events are forwarded to onTool. It retries on timeouts
// (up to MaxAPIRetry) and backs off on rate limits up to MaxRateLimitBackoff.
func Drive(
	ctx context.Context,
	backend sdk.Backend,
	agent sdk.Agent,
	prompt string,
	maxTurns int,
	onToolStart func(name string),
	onTool ToolHook,
) error {
	maxRetry := MaxAPIRetry
	rateBackoff := InitialRateBackoff
	var lastRate error

	for {
		err := runOnce(ctx, backend, agent, prompt, maxTurns, onToolStart, onTool)
		if err == nil {
			render.Output("\n\n")
			return nil
		}

		var timeoutErr *sdk.TimeoutError
		var rateErr *sdk.RateLimitError
		switch {
		case errors.As(err, &timeoutErr):
			if maxRetry == 0 {
				slog.Error("max retries for timeout reached", "err", err)
				return err
			}
			maxRetry--
			slog.Warn("backend timeout; retrying", "remaining", maxRetry, "err", err)

		case errors.As(err, &rateErr):
			lastRate = err
			if rateBackoff >= MaxRateLimitBackoff {
				return &sdk.TimeoutError{Msg: "max rate limit backoff reached: " + err.Error()}
			}
			rateBackoff *= 2
			if rateBackoff > MaxRateLimitBackoff {
				rateBackoff = MaxRateLimitBackoff
			}
			slog.Warn("rate limited; backing off", "backoff", rateBackoff)
			select {
			case <-time.After(rateBackoff):
			case <-ctx.Done():
				return ctx.Err()
			}

		default:
			return err
		}
		_ = lastRate
	}
}

// runOnce consumes one full stream, enforcing a per-event idle timeout.
func runOnce(
	ctx context.Context,
	backend sdk.Backend,
	agent sdk.Agent,
	prompt string,
	maxTurns int,
	onToolStart func(name string),
	onTool ToolHook,
) error {
	st, err := backend.RunStreamed(ctx, agent, prompt, maxTurns)
	if err != nil {
		return err
	}
	defer st.Close()

	for {
		ev, err := recvWithIdleTimeout(ctx, st, IdleTimeout)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch e := ev.(type) {
		case sdk.TextDelta:
			render.Output(e.Text)
		case sdk.ToolEnd:
			if onToolStart != nil {
				onToolStart(e.ToolName)
			}
			if onTool != nil {
				onTool(e.ToolName, e.Text)
			}
		}
	}
}

// recvResult bundles a Recv outcome for the idle-timeout select.
type recvResult struct {
	ev  sdk.StreamEvent
	err error
}

// recvWithIdleTimeout reads the next event, returning a TimeoutError if no
// event arrives within idle.
func recvWithIdleTimeout(ctx context.Context, st sdk.Stream, idle time.Duration) (sdk.StreamEvent, error) {
	ch := make(chan recvResult, 1)
	go func() {
		ev, err := st.Recv()
		ch <- recvResult{ev, err}
	}()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.ev, r.err
	case <-timer.C:
		return nil, &sdk.TimeoutError{Msg: "backend stream idle timeout"}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
