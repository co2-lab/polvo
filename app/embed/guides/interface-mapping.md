# Guide: Interface Mapping

## Purpose

This guide defines how to identify, classify, and document the interfaces of an existing project so that Polvo can orchestrate it effectively. The output of this process is:

1. A list of source files that define interfaces, grouped by type
2. A `polvo.yaml` `interfaces` section with glob patterns for each group
3. A `.spec.md` and a `.feature` file for each identified interface file

## What is an Interface

An interface is any point of interaction between the user (or an external system) and the application. It is not a type or a route — it is the **source file** that defines those interactions.

| Application type            | Interfaces are                                               |
| --------------------------- | ------------------------------------------------------------ |
| **Web portal**              | Pages the user views and navigates                           |
| **Mobile**                  | Screens the user moves through                               |
| **REST API**                | Files that register routes/handlers                          |
| **Desktop**                 | Windows and views the user sees                              |
| **CLI**                     | Files that define commands the user can execute              |
| **Package / SDK**           | Files that export public classes, functions, and types       |
| **WebSocket / Real-time**   | Files that define events and channels clients can subscribe  |
| **Plugin / Extension**      | Files that expose hooks and extension points                 |
| **Webhook**                 | Files that define endpoints external systems call            |
| **Background job / Worker** | Files that define tasks and queues                           |
| **Embedded / IoT**          | Files that define signals, protocols, and peripherals        |

## Unit of Documentation

The unit of documentation is the **source file**, not the individual route, command, or event.

```
source file → source file.spec.md → source file.feature
```

One file that registers 5 HTTP routes produces one `.spec.md` covering all 5 routes and one `.feature` with the corresponding scenarios.

## Identification Process

### Step 1 — Understand the project structure

Read the entrypoints: `main.go`, `index.ts`, `app.py`, or equivalent. Follow imports to find files that register routes, commands, event handlers, or expose public APIs.

### Step 2 — Enumerate interface files

For each file found, record:
- **Path** — relative to project root
- **Type** — one of: `http`, `websocket`, `sse`, `cli`, `tui`, `webhook`, `git-hook`, `sdk`, `worker`, `plugin`
- **What it defines** — brief description (e.g. "registers filesystem CRUD routes", "defines /task and /model slash commands")
- **Glob pattern** — a pattern that matches this file and others of the same group

### Step 3 — Group by interface type

Files of the same type and naming convention belong to the same interface group in `polvo.yaml`. Examples:

| Group name    | Pattern example                   | Type      |
| ------------- | --------------------------------- | --------- |
| `http`        | `internal/server/*_handler.go`    | HTTP      |
| `websocket`   | `internal/server/terminal*.go`    | WebSocket |
| `cli`         | `cmd/**/main.go`                  | CLI       |
| `tui`         | `internal/tui/*.go`               | TUI       |
| `webhook`     | `cmd/server/main.go`              | Webhook   |

### Step 4 — Write polvo.yaml interfaces section

```yaml
interfaces:
  http:
    patterns:
      - 'internal/server/*_handler.go'
      - 'internal/server/handlers.go'
    derived:
      spec: '{{.Dir}}/{{.Name}}.spec.md'
      features: '{{.Dir}}/{{.Name}}.feature'
  websocket:
    patterns:
      - 'internal/server/terminal.go'
    derived:
      spec: '{{.Dir}}/{{.Name}}.spec.md'
      features: '{{.Dir}}/{{.Name}}.feature'
```

## Quality Criteria

- Every interface file in the project MUST appear in at least one pattern group
- Patterns MUST be specific enough to not match non-interface files (config, utils, models, tests)
- One group per interface type — do not mix HTTP and WebSocket in the same group
- If a file defines both HTTP routes and SSE, assign it to the dominant type or split it
- The output is the `interfaces` yaml block, ready to merge into `polvo.yaml` — nothing else
