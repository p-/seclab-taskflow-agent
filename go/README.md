<!--
SPDX-FileCopyrightText: GitHub, Inc.
SPDX-License-Identifier: MIT
-->

# SecLab Taskflow Agent — Go port

A Go implementation of the [SecLab Taskflow Agent](../README.md): an
MCP-enabled, YAML-driven runner for declarative agentic workflows. It reads the
**same YAML grammar** as the Python version (taskflows, personalities,
toolboxes, model configs, prompts), so existing files under `examples/` and the
bundled `seclab_taskflow_agent.*` resources run unchanged.

This is an **MVP**: a single OpenAI backend behind a pluggable interface, with
MCP client support over stdio and streamable HTTP.

## Quick start

```bash
cd go
make build                       # builds ./bin/seclab-taskflow-agent

export AI_API_TOKEN=...           # or COPILOT_TOKEN
export AI_API_ENDPOINT=https://api.openai.com/v1   # optional; defaults to Copilot

# Run a taskflow (dotted resource name; see "Resource resolution" below):
SECLAB_TASKFLOW_PATH=$PWD/.. ./bin/seclab-taskflow-agent -t examples.taskflows.echo

# Run a single personality with an inline prompt:
SECLAB_TASKFLOW_PATH=$PWD/.. ./bin/seclab-taskflow-agent \
  -p examples.personalities.echo "echo hello"

# Resume a checkpointed session:
./bin/seclab-taskflow-agent --resume <session-id>
```

## CLI flags

| Flag | Meaning |
|------|---------|
| `-p, --personality` | Personality module path (mutually exclusive with `-t`). |
| `-t, --taskflow` | Taskflow module path (mutually exclusive with `-p`). |
| `-l, --list-models` | List available tool-call models and exit. |
| `-g, --global KEY=VALUE` | Global template variable. Repeatable. |
| `-m, --model-config` | Model configuration module path. |
| `--resume <id>` | Resume a previous session. |
| `-d, --debug` | Show full error details (or set `TASK_AGENT_DEBUG=1`). |
| `[prompt...]` | Trailing prompt text. |

## Resource resolution

Dotted resource names such as `examples.taskflows.echo` map to
`examples/taskflows/echo.yaml`. Files are resolved in this order:

1. Each colon-separated root in `$SECLAB_TASKFLOW_PATH`.
2. The current working directory.
3. The directory containing the executable.
4. An **embedded snapshot** of the Python package's bundled toolboxes and
   personalities (the `seclab_taskflow_agent.*` namespace), so those resolve
   even without the Python source tree.

Refresh the embedded snapshot after changing the Python bundled YAMLs:

```bash
make sync-embedded
```

## Environment variables

| Variable | Purpose |
|----------|---------|
| `AI_API_TOKEN` / `COPILOT_TOKEN` | API token. |
| `AI_API_ENDPOINT` | API base URL (default: `https://api.githubcopilot.com`). |
| `SECLAB_TASKFLOW_BACKEND` | Force a backend by name (default `openai`). |
| `SECLAB_TASKFLOW_PATH` | Colon-separated resource search roots. |
| `TASKFLOW_ENV_DENYLIST` | Comma-separated env vars withheld from MCP subprocesses. |
| `LOG_DIR` | Override the log directory. |
| `TASK_AGENT_DEBUG` | `1`/`true`/`yes` enables full error output. |

A `.env` file in the working directory is loaded automatically.

## Backends

The runner drives backends behind the `sdk.Backend` interface. Two backends
ship today:

- **`openai`** (default) — supports both the **Chat Completions** and
  **Responses** APIs (streaming, with MCP tool calling), selected per
  task/model via the `api_type` field.
- **`anthropic_sdk`** — drives the native Anthropic **Messages** API
  (`/v1/messages`) via the [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go)
  SDK. Supports streaming, MCP tool calling, adaptive extended thinking with a
  configurable `reasoning.effort` (`low`, `medium`, `high`, `max`),
  `temperature`/`top_p`, automatic ephemeral prompt caching (opt out with
  `prompt_caching: false`), and `exclude_from_context`. Handoffs are not
  supported. Designed for CAPI's Anthropic endpoint; providers in the registry
  authenticate with `Authorization: Bearer` (not `x-api-key`).

Select a backend with the `backend:` field. Selection precedence:

1. Per-task `backend:` in the task's `model_settings`.
2. Per-model `backend:` in the model config's `model_settings`.
3. Top-level `backend:` in the model config.
4. `SECLAB_TASKFLOW_BACKEND`.
5. `openai`.

```yaml
seclab-taskflow-agent:
  version: "1.0"
  filetype: model_config
models:
  code_analysis: claude-opus-4.7
model_settings:
  code_analysis:
    api_type: messages
    backend: anthropic_sdk
    reasoning:
      effort: high
```

### Adding a backend

1. Create `internal/sdk/<name>/` implementing `sdk.Backend`
   (`Name`, `Validate`, `Build`, `RunStreamed`).
2. Call `sdk.Register(&Backend{})` from an `init()` function.
3. Add a blank import of the package where backends are wired (see
   `internal/runner/deploy.go`).

No runner changes are required.

## Feature parity with the Python version

| Feature | Status |
|---------|--------|
| YAML grammar (all filetypes, version normalisation, `extra` keys) | ✅ |
| Jinja2 templating (`{{ globals }}`, `{{ inputs }}`, `{{ result }}`, `{{ env() }}`, `{% include %}`, strict-undefined) | ✅ (via gonja) |
| Single-agent tasks | ✅ |
| Shell `run:` tasks | ✅ |
| `repeat_prompt` iteration | ✅ |
| `globals` / `inputs` / task-scoped `env` | ✅ |
| `must_complete`, `max_steps`, `headless`, `blocked_tools` | ✅ |
| `toolboxes` override, reusable tasks (`uses:`) | ✅ |
| MCP over stdio and streamable HTTP (incl. local-process launch) | ✅ |
| Tool-name compression + namespacing, interactive `confirm:` | ✅ |
| Session checkpoint / `--resume`, task retry/backoff | ✅ |
| OpenAI backend (Chat Completions streaming + tools) | ✅ |
| OpenAI backend (Responses API streaming + tools) | ✅ |
| Anthropic backend (Messages API streaming + tools, `api_type: messages`) | ✅ |
| Provider registry (Copilot / GitHub Models / OpenAI / custom) | ✅ |
| Multi-personality handoffs | ❌ rejected at validation |
| `async:` / `async_limit` parallel fan-out | ✅ |
| MCP over SSE | ❌ fails with a clear error |
| `exclude_from_context` | ✅ |
| `api_type: messages` (Anthropic Messages API) | ✅ |
| Model listing (`-l`) | ✅ |
| Watchdog | ❌ not ported |
| `copilot_sdk` backend | ❌ not ported |

Unsupported features fail fast with an explicit message rather than silently
misbehaving.

## Development

```bash
make test        # unit tests
make test-e2e    # end-to-end echo test (needs ../.venv with the package)
make vet         # go vet
make lint        # golangci-lint if installed
```

## Layout

```
cmd/seclab-taskflow-agent/   entry point
internal/grammar/            YAML models + validators (ports models.py)
internal/loader/             resource loader + search path + embed (available_tools.py)
internal/tmpl/               gonja Jinja2 rendering (template_utils.py, env_utils.py)
internal/envutil/            TmpEnv + denylist (env_utils.py, mcp_transport.py)
internal/capi/               provider/token/endpoint registry (capi.py)
internal/sdk/                backend interface, errors, registry (sdk/base.py)
internal/sdk/openai/         OpenAI backend + agent loop
internal/sdk/anthropic/      Anthropic Messages backend + agent loop
internal/mcp/                MCP params, namespacing, lifecycle (mcp_*.py)
internal/prompt/             system-prompt builder (mcp_prompt.py)
internal/shell/              shell run-task executor (shell_utils.py)
internal/session/            checkpoint/resume (session.py)
internal/stream/             stream driver w/ retry/backoff (_stream.py)
internal/render/             stdout + log mirror (render_utils.py)
internal/paths/              data/log dirs (path_utils.py)
internal/runner/             orchestration (runner.py)
internal/cli/                cobra CLI (cli.py)
```
