<div align="center">
  <img src="https://raw.githubusercontent.com/co2-lab/polvo/main/site/logo.svg" width="120" alt="Polvo" /><br><br>
  <img src="https://raw.githubusercontent.com/co2-lab/polvo/main/site/title.svg" width="200" alt="polvo" />

  <br><br>

  <p>Desktop editor rethought for working with AI agents.<br>Write specs. Run agents. Ship faster.</p>

  <br>

  [![License](https://img.shields.io/badge/License-ELv2-00ffab?style=flat-square)](LICENSE)
  [![Release](https://img.shields.io/github/v/release/co2-lab/polvo?style=flat-square&color=00c4ff)](https://github.com/co2-lab/polvo/releases)
  [![Go](https://img.shields.io/badge/Go-1.25-7b61ff?style=flat-square&logo=go&logoColor=white)](https://go.dev)
  [![Platform](https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-grey?style=flat-square)](#installation)

  <br>

  [Website](https://polvo.co2lab.io) · [Releases](https://github.com/co2-lab/polvo/releases) · [Issues](https://github.com/co2-lab/polvo/issues)

</div>

---

## What is Polvo?

Polvo is a **spec-first desktop editor** for AI-augmented development. Instead of jumping straight to code, you write a spec — then specialized AI agents generate, verify, and refine the implementation.

- **Write specs** in markdown alongside your code
- **Run agents** that understand your project's context and conventions
- **Review output** directly in the built-in editor and terminal
- **No cloud lock-in** — your API keys, your machine, your data

---

## Installation

**macOS**
```bash
brew install --cask co2-lab/tap/polvo
```

**Windows**
```powershell
winget install co2-lab.Polvo
```

**Linux** — download `.AppImage` or `.deb` from [releases](https://github.com/co2-lab/polvo/releases/latest).

---

## Quick Start

1. Open Polvo and point it at a project folder
2. Create a spec file — e.g. `src/auth/login.spec.md`
3. Describe what you want to build in plain language
4. Run an agent from the sidebar
5. Review the output in the editor

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

<details>
<summary>Supported providers</summary>

| Provider | Type | Env var |
|---|---|---|
| Ollama | `ollama` | — |
| Claude | `claude` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenAI-compatible | `openai-compatible` | `API_KEY` |

</details>

<details>
<summary>Environment variables</summary>

| Variable | Default | Description |
|---|---|---|
| `POLVO_ROOT` | `cwd` | Project root |
| `POLVO_IDE_PORT` | `7373` | HTTP server port |
| `SHELL` | `/bin/sh` | Terminal shell (Unix) |
| `COMSPEC` | `cmd.exe` | Terminal shell (Windows) |

</details>

---

## Architecture

```
polvo/
  app/       # Go backend — HTTP server, agents, file system API
  ui/        # React frontend — editor, terminal, panels
  desktop/   # Tauri wrapper — native window, sidecar
```

---

## Development

```bash
make dev        # Desktop app (Tauri + hot reload)
make web-dev    # Web only, no Tauri
make app-dev    # Backend only
make test       # Run tests
make build      # Production build
```

---

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR.

---

## License

[Elastic License 2.0](LICENSE) — free to use, modify, and self-host.

<div align="center">
  <br>
  <sub>Built by <a href="https://github.com/co2-lab">co2-lab</a></sub>
</div>
