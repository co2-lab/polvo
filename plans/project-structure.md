# Plano de Reestruturação do Projeto Polvo

## Contexto

O projeto começou como uma TUI em Go, evoluiu para um dashboard web no browser, e hoje é um app desktop Tauri com backend Go sidecar. A estrutura atual carrega resíduos de cada fase. Este documento propõe uma estrutura ideal para o estado atual e futuro do projeto.

## O que o projeto é hoje

- **App desktop** (Tauri) com webview React
- **Backend Go** rodando como sidecar (processo separado iniciado pelo Tauri)
- Comunicação frontend ↔ backend via **HTTP + SSE** (localhost)
- Terminal PTY via **WebSocket**
- Editor de código, explorador de arquivos, diff viewer, terminal integrado
- Detecção e orquestração de **CLIs de IA** (Claude, Copilot, Gemini, etc.)
- **Agentes LLM** com pipeline de reações

---

## Estrutura Ideal

```
polvo/
├── app/                          # Backend Go (sidecar)
│   ├── cmd/
│   │   ├── polvo/            # Entrypoint do sidecar (servidor HTTP)
│   │   │   └── main.go
│   │   └── polvo-server/         # Entrypoint do webhook server (GitHub)
│   │       └── main.go
│   │
│   ├── internal/
│   │   ├── agent/                # Orquestração de agentes LLM
│   │   │   ├── agent.go          # Interface + LLMAgent
│   │   │   ├── executor.go       # Executor (run agent, handle tools)
│   │   │   └── conversation.go   # Loop de conversa
│   │   │
│   │   ├── config/               # Carregamento de configuração
│   │   │   ├── config.go         # Structs + defaults
│   │   │   ├── loader.go         # koanf + yaml
│   │   │   └── validator.go      # go-playground/validator
│   │   │
│   │   ├── provider/             # Adaptadores LLM
│   │   │   ├── provider.go       # Interface Provider
│   │   │   ├── registry.go       # Registry pattern
│   │   │   ├── claude.go         # Anthropic Claude
│   │   │   ├── gemini.go         # Google Gemini
│   │   │   └── ollama.go         # Ollama (local)
│   │   │
│   │   ├── tool/                 # Ferramentas para LLM
│   │   │   ├── tool.go           # Interface Tool
│   │   │   ├── fs.go             # read, write, edit, glob, ls
│   │   │   ├── shell.go          # bash execution
│   │   │   └── grep.go           # search
│   │   │
│   │   ├── pipeline/             # Scheduler de reações
│   │   │   ├── scheduler.go
│   │   │   └── reaction.go
│   │   │
│   │   ├── guide/                # Resolução de guias
│   │   │   ├── resolver.go
│   │   │   └── loader.go
│   │   │
│   │   ├── template/             # Renderização de prompts
│   │   │   └── renderer.go
│   │   │
│   │   ├── watcher/              # File watcher
│   │   │   └── watcher.go        # fsnotify + debounce
│   │   │
│   │   ├── clidetect/            # Detecção de CLIs de IA
│   │   │   └── detect.go
│   │   │
│   │   ├── report/               # Geração de relatórios
│   │   │   └── report.go
│   │   │
│   │   └── server/               # HTTP server (dashboard API)
│   │       ├── server.go         # Setup, roteamento
│   │       ├── events.go         # SSE event bus
│   │       ├── terminal.go       # WebSocket PTY
│   │       ├── projects.go       # CRUD projetos
│   │       ├── fs.go             # Filesystem endpoints
│   │       ├── agents.go         # Agentes endpoints
│   │       ├── clis.go           # CLIs detectados
│   │       └── status.go         # Health + info
│   │
│   ├── embed/                    # Assets embedded no binário
│   │   ├── embed.go
│   │   ├── config.yaml           # Config padrão
│   │   ├── guides/               # Guias de contexto
│   │   └── prompts/              # Templates de prompt
│   │
│   ├── go.mod
│   ├── go.sum
│   └── Makefile
│
├── ui/                           # Frontend React (Tauri webview)
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── index.css
│   │   │
│   │   ├── components/
│   │   │   ├── common/           # Componentes genéricos reutilizáveis
│   │   │   │   ├── FloatingModal.tsx
│   │   │   │   ├── ConfirmDialog.tsx
│   │   │   │   └── ...
│   │   │   │
│   │   │   ├── layout/           # Shell e estrutura da janela
│   │   │   │   ├── WorkspaceTabs.tsx
│   │   │   │   ├── SidePanel.tsx
│   │   │   │   ├── Dock.tsx
│   │   │   │   └── DockManagerModal.tsx
│   │   │   │
│   │   │   ├── workspace/        # Área de trabalho (painéis)
│   │   │   │   ├── WorkspaceArea.tsx
│   │   │   │   ├── LayoutRenderer.tsx
│   │   │   │   └── PanelFrame.tsx
│   │   │   │
│   │   │   ├── editor/           # Editor de código
│   │   │   │   ├── EditorPane.tsx
│   │   │   │   ├── TabBar.tsx
│   │   │   │   └── SaveAsModal.tsx
│   │   │   │
│   │   │   ├── terminal/         # Terminal PTY
│   │   │   │   └── TerminalPanel.tsx
│   │   │   │
│   │   │   ├── explorer/         # Explorador de arquivos
│   │   │   │   ├── ExplorerPanel.tsx
│   │   │   │   └── FileTree.tsx
│   │   │   │
│   │   │   ├── diff/             # Diff viewer
│   │   │   │   └── DiffPanel.tsx
│   │   │   │
│   │   │   ├── settings/         # Janela de configurações
│   │   │   │   ├── SettingsModal.tsx
│   │   │   │   └── ProjectConfigModal.tsx
│   │   │   │
│   │   │   ├── welcome/          # Tela de boas-vindas / setup
│   │   │   │   ├── WelcomeScreen.tsx
│   │   │   │   ├── InitModal.tsx
│   │   │   │   └── DoctorModal.tsx
│   │   │   │
│   │   │   └── icons/            # Ícones customizados
│   │   │       ├── AIIcons.tsx
│   │   │       └── panelIcon.tsx
│   │   │
│   │   ├── store/                # Estado global (Zustand)
│   │   │   └── useIDEStore.ts
│   │   │
│   │   ├── hooks/                # React hooks customizados
│   │   │   ├── useSSE.ts
│   │   │   ├── useFiles.ts
│   │   │   └── ...
│   │   │
│   │   ├── lib/                  # Utilitários puros (sem React)
│   │   │   ├── fileIcons.ts
│   │   │   ├── fileIconColor.ts
│   │   │   ├── diffUtils.ts
│   │   │   └── i18n/
│   │   │       ├── index.ts
│   │   │       └── locales/
│   │   │           ├── en-US.json
│   │   │           └── pt-BR.json
│   │   │
│   │   └── types/                # TypeScript types
│   │       ├── ide.ts
│   │       └── api.ts
│   │
│   ├── public/
│   ├── index.html
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json
│
├── desktop/                      # Wrapper Tauri (Rust)
│   ├── src/
│   │   ├── main.rs
│   │   └── lib.rs
│   ├── bin/                      # Sidecar binários compilados
│   │   └── polvo-*           # (gerado pelo build)
│   ├── capabilities/
│   │   └── default.json
│   ├── icons/
│   ├── tauri.conf.json
│   ├── Cargo.toml
│   └── Cargo.lock
│
├── docs/                         # Documentação
│   ├── architecture.md           # Este documento (pós-implementação)
│   ├── api.md                    # Referência da API HTTP
│   └── guides.md                 # Como criar guias customizados
│
├── plans/                        # Planos de implementação (este diretório)
│
├── .github/
│   └── workflows/
│       ├── ci.yml                # Testes e lint
│       └── release.yml           # Build e publicação
│
├── Makefile                      # Orquestração geral
├── polvo.yaml                    # Config do projeto (exemplo/dev)
└── README.md
```

---

## O que mudar vs. o que está bem

### Renomear

| Atual | Ideal | Motivo |
|-------|-------|--------|
| `core/` | `app/` | "core" é genérico; "app" descreve o que é |
| `src-tauri/` | `desktop/` | Nome Tauri padrão, não descreve o propósito |
| `core/internal/dashboard/` | `app/internal/server/` | Não é um "dashboard", é o servidor HTTP da API |

### Mover / reorganizar

| O que | De onde | Para onde | Motivo |
|-------|---------|-----------|--------|
| `FileTree.tsx` | `components/filetree/` | `components/explorer/` | Pertence ao explorador conceitualmente |
| `FileEditorPanel.tsx` | `components/panels/` | `components/editor/` | Pertence ao editor |
| `ExplorerPanel.tsx` | `components/panels/` | `components/explorer/` | Idem |
| `terminal.go` (TUI) | `dashboard/` | Deletar | TUI legado, substituído pela web |
| `internal/cli/` | `core/` | Deletar ou mover para `cmd/` | CLI Kong não está mais em uso ativo |

### Deletar (lixo confirmado)

| O que | Motivo |
|-------|--------|
| `/prototype/` | Iteração 1 abandonada |
| `/prototype2/` | Iteração 2 abandonada |
| `/prototype3/` | Iteração 3 abandonada |
| `core/web/index.html` | Substituído pelo ui/ |
| `dashboard/terminal.go` | TUI charmbracelet não está em uso |
| `dashboard/terminal_stub.go` | Idem |
| `internal/cli/` | CLI Kong não é mais o entrypoint |
| `store/ideStore.ts` | Duplicata antiga do useIDEStore |
| `store/IDEContext.tsx` | Context antigo, substituído por Zustand |
| `store/LegacyPropsContext.tsx` | Explicitamente legado |
| `store/persistence.ts` | Lógica movida para useIDEStore |
| `components/sidebar/` | Substituído por SidePanel no layout/ |
| `components/dock/` | Substituído por Dock no layout/ |
| `components/layout/Shell.tsx` | Substituído pela estrutura atual |
| `components/layout/SplitPane.tsx` | Substituído por react-resizable-panels |
| `components/layout/Splitter.tsx` | Idem |
| `components/layout/WorkspaceTabBar.tsx` | Duplicata de WorkspaceTabs |

### Dependências Go a remover

| Pacote | Motivo |
|--------|--------|
| `charmbracelet/bubbletea` | TUI removido |
| `charmbracelet/bubbles` | TUI removido |
| `charmbracelet/glamour` | TUI removido |
| `charmbracelet/huh` | TUI removido |
| `charmbracelet/lipgloss` | TUI removido |
| `alecthomas/kong` | CLI removido (se não tiver mais entrypoint CLI) |

### Dependências frontend a revisar

| Pacote | Status |
|--------|--------|
| `react-arborist` | Verificar se ainda é usado no FileTree |
| `framer-motion` | Apenas motion.button no WorkspaceTabs — talvez remover |
| `js-yaml` | Verificar uso |

---

## Makefile ideal (raiz)

```makefile
# Desenvolvimento desktop (padrão)
make dev        → compila app/ → inicia tauri dev

# Desenvolvimento só web (sem Tauri, abre no browser)
make web-dev    → compila app/ → inicia vite dev

# Testes
make test       → go test ./app/...

# Build de produção
make build      → npm build + go build + tauri build

# Limpeza
make clean      → remove artifacts de build
```

---

## Ordem de execução do plano

1. **Deletar lixo** — protótipos, TUI, CLI, stores legados, componentes duplicados
2. **Limpar dependências Go** — remover charmbracelet, kong se não usado
3. **Renomear `core/` → `app/`** e `src-tauri/` → `desktop/`
4. **Reorganizar `internal/dashboard/` → `internal/server/`**
5. **Reorganizar componentes frontend** — mover FileTree, FileEditorPanel, ExplorerPanel
6. **Atualizar imports** em todos os arquivos afetados
7. **Atualizar Makefile** e `tauri.conf.json` com novos caminhos
8. **Verificar e rodar `make build`** para garantir que tudo compila

---

## Notas

- A estrutura do `ui/src/` já está razoavelmente organizada — as mudanças são pequenas
- O maior ganho é renomear `core/` e `src-tauri/` e deletar o lixo acumulado
- Não há mudança de funcionalidade — é puramente reorganização estrutural
- Após a limpeza, o `go.mod` ficará ~40% menor em dependências transitivas
