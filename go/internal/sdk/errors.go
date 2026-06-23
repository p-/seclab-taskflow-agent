// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package sdk

// Backend error types port the Python ``sdk.errors`` hierarchy. They let the
// runner's retry/backoff loop branch on the failure category regardless of
// which backend produced it.

// CapabilityError indicates a spec used a feature the backend cannot honour.
type CapabilityError struct{ Msg string }

func (e *CapabilityError) Error() string { return e.Msg }

// MaxTurnsError indicates the agent exceeded its turn budget.
type MaxTurnsError struct{ Msg string }

func (e *MaxTurnsError) Error() string { return e.Msg }

// RateLimitError indicates the provider returned a rate-limit response.
type RateLimitError struct{ Msg string }

func (e *RateLimitError) Error() string { return e.Msg }

// TimeoutError indicates a network/stream timeout.
type TimeoutError struct{ Msg string }

func (e *TimeoutError) Error() string { return e.Msg }

// BadRequestError indicates the provider rejected the request (HTTP 400).
type BadRequestError struct{ Msg string }

func (e *BadRequestError) Error() string { return e.Msg }

// UnexpectedError wraps any other backend failure.
type UnexpectedError struct{ Msg string }

func (e *UnexpectedError) Error() string { return e.Msg }
