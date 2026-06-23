// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package openai

import (
	"context"
	"errors"

	oai "github.com/openai/openai-go"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// mapError converts an openai-go / transport error into the backend-neutral
// error taxonomy the runner branches on.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &sdk.TimeoutError{Msg: err.Error()}
	}
	var apiErr *oai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 429:
			return &sdk.RateLimitError{Msg: apiErr.Error()}
		case 400:
			return &sdk.BadRequestError{Msg: apiErr.Error()}
		case 408, 504:
			return &sdk.TimeoutError{Msg: apiErr.Error()}
		default:
			return &sdk.UnexpectedError{Msg: apiErr.Error()}
		}
	}
	return &sdk.UnexpectedError{Msg: err.Error()}
}
