# Contributing to Polvo

## Prerequisites

- Go 1.22+
- Node.js 20+
- Rust + Cargo (only for Tauri desktop builds)

## Running locally

```bash
# Web mode (no Tauri required)
make web-dev

# Desktop mode
make dev
```

## Project structure

```
app/      Go backend (HTTP server, agent orchestration)
ui/       React frontend (Vite + TypeScript)
desktop/  Tauri desktop wrapper
```

## Making changes

1. Fork the repository and create a branch from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes. Run tests before submitting:
   ```bash
   make test
   ```

3. Open a Pull Request against `main`. Describe what the change does and why.

## Guidelines

- Keep PRs focused — one concern per PR
- Backend: follow standard Go conventions (`gofmt`, no unused imports)
- Frontend: TypeScript strict mode, no `any`
- Do not commit `.env`, secrets, or build artifacts
- API keys must always come from environment variables

## Reporting bugs

Open a [GitHub Issue](https://github.com/co2-lab/polvo/issues) with:
- Steps to reproduce
- Expected vs actual behavior
- OS, Go version, Node version

## Security issues

See [SECURITY.md](SECURITY.md) — do not open public issues for vulnerabilities.

## License

By contributing, you agree that your contributions will be licensed under the [Elastic License 2.0](LICENSE).
