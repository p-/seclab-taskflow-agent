// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package openai

import (
	"context"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/sdk"
)

// runResponses executes the Responses API agent loop. It maintains a growing
// input-item list (function call + function-call output items) across turns
// rather than relying on previous_response_id, keeping the loop stateless and
// symmetric with the Chat Completions path. It emits TextDelta and ToolEnd
// events, closing s.events on completion and leaving a terminal error on
// s.errc when one occurs.
func (a *agent) runResponses(ctx context.Context, prompt string, maxTurns int, s *stream) {
	defer close(s.events)

	inputs := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(prompt, responses.EasyInputMessageRoleUser),
	}

	for turn := 0; turn < maxTurns; turn++ {
		params := responses.ResponseNewParams{
			Model:        a.model,
			Instructions: oai.String(a.system),
			Input:        responses.ResponseNewParamsInputUnion{OfInputItemList: inputs},
		}
		if len(a.respTools) > 0 {
			params.Tools = a.respTools
		}
		if a.temp != nil {
			params.Temperature = oai.Float(*a.temp)
		}

		respStream := a.client.Responses.NewStreaming(ctx, params)
		var completed *responses.Response
		for respStream.Next() {
			ev := respStream.Current()
			switch ev.Type {
			case "response.output_text.delta":
				if d := ev.Delta.OfString; d != "" {
					select {
					case s.events <- sdk.TextDelta{Text: d}:
					case <-ctx.Done():
						s.errc <- mapError(ctx.Err())
						_ = respStream.Close()
						return
					}
				}
			case "response.completed":
				r := ev.Response
				completed = &r
			}
		}
		if err := respStream.Err(); err != nil {
			s.errc <- mapError(err)
			return
		}
		if completed == nil {
			return // no terminal response; nothing more to do
		}

		calls := functionCalls(completed.Output)
		if len(calls) == 0 {
			return // natural completion -> io.EOF
		}

		for _, c := range calls {
			if !a.exclude {
				// Echo the model's function call into the next turn's input.
				inputs = append(inputs, responses.ResponseInputItemParamOfFunctionCall(c.arguments, c.callID, c.name))
			}

			result, err := a.dispatchTool(ctx, c.name, c.arguments)
			if err != nil {
				result = "Tool call failed: " + err.Error()
			}
			select {
			case s.events <- sdk.ToolEnd{ToolName: c.name, Text: result}:
			case <-ctx.Done():
				s.errc <- mapError(ctx.Err())
				return
			}
			if !a.exclude {
				inputs = append(inputs, responses.ResponseInputItemParamOfFunctionCallOutput(c.callID, result))
			}
		}
		if a.exclude {
			return
		}
	}

	s.errc <- &sdk.MaxTurnsError{Msg: "maximum turns exceeded"}
}

// responsesCall is a function call extracted from a Responses API output.
type responsesCall struct {
	name      string
	callID    string
	arguments string
}

// functionCalls collects the function_call items from a response output list.
func functionCalls(output []responses.ResponseOutputItemUnion) []responsesCall {
	var calls []responsesCall
	for _, item := range output {
		if item.Type == "function_call" {
			calls = append(calls, responsesCall{
				name:      item.Name,
				callID:    item.CallID,
				arguments: item.Arguments,
			})
		}
	}
	return calls
}
