# Polvo

[![License: ELv2](https://img.shields.io/badge/License-Elastic_v2-lightblue.svg)](LICENSE)

AI agent orchestrator for spec-first projects — with a built-in web IDE.

Polvo watches your repository for file changes and triggers specialized AI agents to generate, verify, and fix project artifacts. Every change produced by an agent is delivered via Pull Request. A reviewer agent checks the PR using its own guides. The Git history is the backbone — nothing is changed directly on the main branch.

The IDE runs as a local web server (embedded in the binary) and opens in Tauri as a desktop app or in the browser for web-only mode.

---

## Architecture

```
polvo/
  app/          # Go backend (HTTP server + agent orchestration)
  ui/           # React frontend (Vite + TypeScript)
  desktop/      # Tauri desktop wrapper
```

The Go binary (`polvo`) embeds the compiled UI and serves it at `http://localhost:7373`. Tauri wraps it as a native desktop app with a sidecar process.

---

## Development

### Prerequisites

- Go 1.22+
- Node.js 20+
- Rust + Cargo (for Tauri desktop)
- `npm install -g @tauri-apps/cli` or use `npx tauri`

### Desktop app (Tauri)

```bash
make dev
```

Builds the Go backend, copies the binary into the Tauri sidecar path, and starts `tauri dev` with the Vite dev server.

### Web only (no Tauri)

```bash
make web-dev
```

Starts the Go server on port 7373 and the Vite dev server with proxy. Open `http://localhost:5173`.

### Backend only

```bash
make app-dev
```

Runs the Go server directly. Useful for backend development without the UI.

---

## Build

```bash
make build
```

Builds the UI, compiles the Go binary, and runs `tauri build` to produce platform installers in `desktop/target/release/bundle/`.

---

## Configuration

Place a `polvo.yaml` in your project root:

```yaml
project:
  name: "my-project"

providers:
  default:
    type: claude
    api_key: "${ANTHROPIC_API_KEY}"
    default_model: "claude-sonnet-4-6"
  local:
    type: ollama
    base_url: "http://localhost:11434"
    default_model: "codellama:13b"

interfaces:
  patterns:
    - "src/**/*.tsx"
    - "api/handlers/**/*.go"
  derived:
    spec:     "{{dir}}/{{name}}.spec.md"
    features: "{{dir}}/{{name}}.feature"
    tests:    "{{dir}}/{{name}}.test.{{ext}}"
```

API keys are always referenced via environment variables — never hardcoded.

### Supported providers

| Provider | Type | Required env var |
|---|---|---|
| Ollama | `ollama` | — (local, default `http://localhost:11434`) |
| Claude | `claude` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenAI-compatible | `openai-compatible` | `API_KEY` |

---

## Tests

```bash
make test
```

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `POLVO_ROOT` | `cwd` | Project root directory (set automatically by Tauri) |
| `POLVO_IDE_PORT` | `7373` | HTTP server port |
| `SHELL` | `/bin/sh` | Shell used for the integrated terminal (Unix) |
| `COMSPEC` | `cmd.exe` | Shell used for the integrated terminal (Windows) |
