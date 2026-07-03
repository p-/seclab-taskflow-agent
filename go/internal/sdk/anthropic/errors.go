// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package anthropic

import (
	"context"
	"errors"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// mapError converts an anthropic-sdk-go / transport error into the
// backend-neutral error taxonomy the runner branches on. It mirrors the
// Python anthropic_sdk backend's exception mapping: 429 -> rate limit,
// timeouts -> timeout, other 4xx -> bad request, everything else ->
// unexpected.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &sdk.TimeoutError{Msg: err.Error()}
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 429:
			return &sdk.RateLimitError{Msg: apiErr.Error()}
		case apiErr.StatusCode == 408 || apiErr.StatusCode == 504:
			return &sdk.TimeoutError{Msg: apiErr.Error()}
		case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500:
			return &sdk.BadRequestError{Msg: apiErr.Error()}
		default:
			return &sdk.UnexpectedError{Msg: apiErr.Error()}
		}
	}
	return &sdk.UnexpectedError{Msg: err.Error()}
}
