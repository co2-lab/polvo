# Plano de Implementação — IDE UX

> Referência: `docs/ide-ux.md`
> Base de código atual: React 19 + TypeScript + Vite + Monaco Editor
> Layout atual: `Shell.tsx` — sidebar fixa + editor + painel inferior com tabs fixas

---

## Visão Geral das Fases

| Fase | Foco | Entrega |
|---|---|---|
| 0 | Fundação | Design tokens CSS, estrutura de tipos, layout shell |
| 1 | Motor de Paineis | Split engine, splitter resize, paineis básicos |
| 2 | Dock (Toolbox) | Dock nativo + CLIs de IA detectados |
| 3 | Containers (Workspaces) | Tab bar, múltiplos workspaces |
| 4 | Drag-and-Drop | Drop zones, reposicionamento, cross-container |
| 5 | Sidebars de Controle | Project sidebar, Panel Manager |
| 6 | Janela de Configurações | Modal settings, seções Geral e Temas |
| 7 | Configurações Avançadas | Atalhos, Agentes, Providers, Avançado |
| 8 | Marketplace | UI de extensões, manifesto, hot-load de temas |
| 9 | Persistência | Salvar/restaurar layout entre sessões |

Cada fase é independente e entregável — a IDE funciona ao final de cada uma.

---

## Fase 0 — Fundação

> **Objetivo:** preparar o terreno sem quebrar o que existe. Nenhuma feature nova visível.

### 0.1 Design Tokens CSS

Criar `ui/src/index.css` (ou arquivo separado `ui/src/tokens.css`) com todas as variáveis CSS documentadas em `ide-ux.md §9`:

```css
:root {
  --primary: #00ffab;
  --accent: #ff5e8e;
  --user: #5c9cf5;
  --error: #e06c75;
  --success: #7fd88f;
  --info: #56b6c2;
  --text: #e0e0e0;
  --muted: #4a6a5a;
  --bg: #0d1210;
  --bg-dark: #080d0a;
  --border: #1a3a2a;
  --title-bar: #1a2420;
  --code-bg: #0f1a14;
}
```

Substituir todas as cores hardcoded no CSS existente pelos tokens.

### 0.2 Sistema de Tipos

Criar `ui/src/types/layout.ts` com os tipos fundamentais do motor de paineis:

```ts
type PanelType =
  | 'files' | 'editor' | 'terminal' | 'chat'
  | 'diff' | 'log' | 'agents' | 'panels'
  | 'marketplace' | 'settings' | 'cli'

interface Panel {
  id: string
  type: PanelType
  title: string
  projectId?: string
  visible: boolean
  // para CLIs de IA
  cliCommand?: string
}

type SplitAxis = 'horizontal' | 'vertical'

interface SplitNode {
  axis: SplitAxis
  ratio: number          // 0–1, proporção do primeiro filho
  first: LayoutNode
  second: LayoutNode
}

type LayoutNode = { kind: 'panel'; panelId: string }
                | { kind: 'split'; split: SplitNode }

interface Workspace {
  id: string
  name: string
  layout: LayoutNode | null   // null = vazio
}

interface IDEState {
  workspaces: Workspace[]
  activeWorkspaceId: string
  panels: Record<string, Panel>
  activeProjectId?: string
}
```

### 0.3 Refactor de Shell.tsx

Mover o conteúdo atual de `Shell.tsx` para dentro do novo modelo sem mudar comportamento:

- Criar `ui/src/store/ideStore.ts` — estado global usando `useReducer` + Context (sem lib externa).
- O estado inicial popula um único `Workspace` com o layout existente (sidebar + editor + bottom) representado como `SplitNode`s.
- `Shell.tsx` passa a ser um orquestrador que lê do store e renderiza via `WorkspaceRenderer`.

**Arquivos a criar:**
```
ui/src/
  store/
    ideStore.ts       # IDEState, actions, reducer
    IDEContext.tsx    # Context + Provider + hooks (useIDEState, useIDEDispatch)
  types/
    layout.ts         # tipos definidos acima
```

**Arquivos a modificar:**
- `ui/src/App.tsx` — envolve tudo em `<IDEProvider>`
- `ui/src/components/layout/Shell.tsx` — lê do context em vez de props diretas

**Critério de conclusão:** build passa, comportamento idêntico ao atual.

---

## Fase 1 — Motor de Paineis (Split Engine)

> **Objetivo:** qualquer painel pode ser dividido em dois. Splitter arrastável entre eles.

### 1.1 Componente `SplitPane`

Criar `ui/src/components/layout/SplitPane.tsx`:

- Recebe `axis`, `ratio`, `first: ReactNode`, `second: ReactNode`.
- Renderiza os dois filhos separados por um `Splitter`.
- Ao arrastar o `Splitter`, atualiza `ratio` via callback `onRatioChange`.
- Usa `%` para tamanhos (nunca `px` fixo) para manter proporcionalidade ao redimensionar a janela.

```tsx
<SplitPane
  axis="vertical"
  ratio={0.25}
  onRatioChange={(r) => dispatch({ type: 'UPDATE_RATIO', splitId, ratio: r })}
  first={<FilesPanel />}
  second={<EditorPanel />}
/>
```

### 1.2 Componente `Splitter`

Criar `ui/src/components/layout/Splitter.tsx`:

- `div` com `width: 6px` (vertical) ou `height: 6px` (horizontal), `cursor: col-resize / row-resize`.
- `onMouseDown` → captura movimento global (`mousemove` no `document`) → calcula novo ratio → chama callback.
- Visual: cor `--border` default, `--primary` no hover e durante drag (ver `ide-ux.md §10.1`).

### 1.3 Componente `PanelFrame`

Criar `ui/src/components/layout/PanelFrame.tsx`:

- Wrapper de cada painel com barra de título: ícone + título + botão minimizar + botão fechar.
- Borda superior `2px --primary` quando em foco (foco detectado via click dentro do frame).
- Título em `--text` quando focado, `--muted` quando não.

### 1.4 Renderizador de Layout

Criar `ui/src/components/layout/LayoutRenderer.tsx`:

- Função recursiva que percorre `LayoutNode` e renderiza `SplitPane` ou `PanelFrame`.
- Mapeia `panelId` → componente correto (`FilesPanel`, `EditorPanel`, etc.).

### 1.5 Estado Inicial Padrão (ide-ux.md §12)

No `ideStore.ts`, a função `createInitialState()` gera:

```
SplitNode {
  axis: 'vertical',
  ratio: 0.25,
  first: { kind: 'panel', panelId: 'files-1' },
  second: { kind: 'panel', panelId: 'terminal-1' }
}
```

**Arquivos a criar:**
```
ui/src/components/layout/
  SplitPane.tsx
  Splitter.tsx
  PanelFrame.tsx
  LayoutRenderer.tsx
```

**Critério de conclusão:** abrir a IDE mostra Arquivos (25%) + Terminal (75%), splitter arrastável entre eles.

---

## Fase 2 — Dock (Toolbox)

> **Objetivo:** barra de ferramentas inferior com itens nativos + CLIs detectados.

### 2.1 Componente `Dock`

Criar `ui/src/components/dock/Dock.tsx`:

- Barra horizontal na borda inferior da janela (posição configurável, fase 6).
- Itens dispostos em linha, com ícone e label.
- Separador visual entre grupos: Ferramentas Gerais / CLIs de IA / Extensões.
- **Clique** → chama `dispatch({ type: 'OPEN_PANEL', panelType })` → algoritmo de split (fase 1) adiciona painel.

### 2.2 Componente `DockItem`

Criar `ui/src/components/dock/DockItem.tsx`:

- Ícone (lucide-react) + label.
- Hover: escala 1.1 + tooltip com nome completo.
- Draggable (fase 4 ativa o drag; nesta fase apenas clique funciona).

### 2.3 Detecção de CLIs de IA (Core)

No backend Go (`core/internal/dashboard` ou novo `core/internal/clidetect`):

- Novo endpoint `GET /api/clis` — retorna lista de CLIs encontrados no PATH.
- Lógica: para cada CLI da lista conhecida, executa `exec.LookPath(cli)`. Se encontrar, inclui na resposta.
- Lista inicial: `claude`, `gemini`, `gh` (para `gh copilot`), `aider`, `continue`.
- Resposta:

```json
[
  { "id": "claude", "label": "Claude", "command": "claude" },
  { "id": "gemini", "label": "Gemini", "command": "gemini" }
]
```

### 2.4 Hook `useCLIs`

Criar `ui/src/hooks/useCLIs.ts`:

- `GET /api/clis` no mount.
- Retorna lista de CLIs disponíveis.
- `Dock` consome e renderiza o grupo de CLIs dinamicamente.

### 2.5 Painel Terminal com CLI pré-carregado

Modificar `TerminalPanel.tsx` para aceitar `initialCommand?: string`:

- Se fornecido, executa o comando imediatamente ao montar o componente (envia via `POST /terminal/input`).
- O título do `PanelFrame` mostra `"Terminal — [label]"` quando aberto via CLI do Dock.

**Arquivos a criar:**
```
ui/src/components/dock/
  Dock.tsx
  DockItem.tsx
ui/src/hooks/
  useCLIs.ts
core/internal/clidetect/
  detect.go    # LookPath para cada CLI da lista
```

**Arquivos a modificar:**
- `core/internal/dashboard/server.go` — registrar `GET /api/clis`
- `ui/src/components/terminal/TerminalPanel.tsx` — aceitar `initialCommand`
- `ui/src/components/layout/Shell.tsx` — renderizar `<Dock>` na borda inferior

**Critério de conclusão:** Dock visível com todos os itens nativos. CLIs instalados aparecem. Clicar em um item abre o painel correspondente. Clicar num CLI abre Terminal já com o CLI rodando.

---

## Fase 3 — Containers (Workspaces)

> **Objetivo:** múltiplas áreas de trabalho navegáveis por abas no topo.

### 3.1 Componente `WorkspaceTabBar`

Criar `ui/src/components/layout/WorkspaceTabBar.tsx`:

- Renderiza uma aba por `Workspace` no `IDEState.workspaces`.
- Aba ativa: destaque mint, texto `--text`.
- Aba inativa: texto `--muted`, fundo `--title-bar`.
- Botão `+` ao final para criar novo workspace (`dispatch({ type: 'ADD_WORKSPACE' })`).
- Duplo clique na aba → input inline para renomear.
- Botão `×` na aba → fechar (se houver paineis, confirmar).
- Drag entre abas → reordenar.

### 3.2 Renderização por Workspace Ativo

`Shell.tsx` passa a renderizar apenas o `Workspace` com `id === IDEState.activeWorkspaceId`. Troca de aba → atualiza `activeWorkspaceId` → re-render.

### 3.3 Isolamento de Estado por Workspace

- Cada workspace tem seu próprio `LayoutNode` — o redimensionamento de splitters em um não afeta os outros.
- Paineis são globais (existem em `IDEState.panels`) mas referenciados por ID nos layouts dos workspaces — um mesmo painel pode estar em apenas um workspace.

**Arquivos a criar:**
```
ui/src/components/layout/
  WorkspaceTabBar.tsx
```

**Arquivos a modificar:**
- `ui/src/components/layout/Shell.tsx` — adicionar `<WorkspaceTabBar>` acima do conteúdo
- `ui/src/store/ideStore.ts` — actions: `ADD_WORKSPACE`, `REMOVE_WORKSPACE`, `RENAME_WORKSPACE`, `SWITCH_WORKSPACE`, `REORDER_WORKSPACES`

**Critério de conclusão:** criar, renomear, fechar e trocar entre workspaces via tab bar. Cada workspace mantém seu próprio layout de paineis de forma independente.

---

## Fase 4 — Drag-and-Drop de Paineis

> **Objetivo:** arrastar painel pelo título e soltar em drop zone de outro painel ou na tab bar de outro workspace.

### 4.1 Infraestrutura de DnD

Usar a API nativa de drag do browser (`draggable`, `onDragStart`, `onDragOver`, `onDrop`) — sem biblioteca. Contexto de drag global via `DragContext`:

```ts
interface DragState {
  dragging: boolean
  panelId: string | null
  sourceWorkspaceId: string | null
}
```

### 4.2 Drop Zones em `PanelFrame`

Modificar `PanelFrame.tsx`:

- Quando `DragContext.dragging === true` e o frame não é o painel sendo arrastado: renderizar overlay com 5 zonas (`CIMA`, `BAIXO`, `ESQ`, `DIR`, `CENTRO`).
- Zona detectada por posição relativa do cursor dentro do frame durante `onDragOver`.
- Visual: overlay mint 20% opacidade na zona ativa + borda mint `2px` na borda correspondente (ver `ide-ux.md §10.2`).
- `onDrop`: `dispatch({ type: 'MOVE_PANEL', panelId, targetPanelId, zone })`.

### 4.3 Drop Zones — Lógica no Reducer

`MOVE_PANEL` no reducer:

1. Remove o `panelId` do `LayoutNode` atual (substituindo o nó pela outra "metade" do split).
2. Insere um novo `SplitNode` no nó do `targetPanelId`, com o painel arrastado no lado correspondente à `zone`.
3. Calcula `ratio` inicial: 0.5 para ESQ/DIR/CIMA/BAIXO, empilha para CENTRO (fase futura).

### 4.4 Drag para outro Workspace

`WorkspaceTabBar`: ao arrastar um painel sobre uma aba de workspace diferente, após 600ms de hover (`setTimeout` + `clearTimeout` no `onDragLeave`), ativa esse workspace. `onDrop` na aba insere o painel no workspace alvo usando o posicionamento inteligente (fase 1.4).

### 4.5 Drag a partir do Dock

`DockItem` ganha `draggable`. `onDragStart` registra no `DragContext` que a origem é o Dock (não um painel existente). Drop sobre um `PanelFrame` → cria novo painel na zone indicada.

**Arquivos a criar:**
```
ui/src/store/
  DragContext.tsx
```

**Arquivos a modificar:**
- `ui/src/components/layout/PanelFrame.tsx` — drop zones overlay
- `ui/src/components/layout/WorkspaceTabBar.tsx` — drop entre workspaces
- `ui/src/components/dock/DockItem.tsx` — draggable
- `ui/src/store/ideStore.ts` — action `MOVE_PANEL`

**Critério de conclusão:** arrastar painel pelo título e soltar em qualquer zona de qualquer painel produz o split correto. Arrastar para aba de outro workspace move o painel. Arrastar item do Dock cria novo painel via drop.

---

## Fase 5 — Sidebars de Controle

> **Objetivo:** Project Sidebar e Panel Manager conforme `ide-ux.md §3` e `§6`.

### 5.1 Project Sidebar

Criar `ui/src/components/sidebar/ProjectSidebar.tsx`:

- Lista projetos carregados (`IDEState.projects`, populado via `GET /api/status` e SSE).
- Projeto ativo (cujo painel está em foco) marcado com `●` mint.
- Botão `[–]` por projeto → `dispatch({ type: 'MINIMIZE_PROJECT', projectId })` — oculta todos os paineis associados ao projeto em todos os workspaces.
- Projeto minimizado com indicador visual distinto.
- Bidireccionalidade: foco em painel → atualiza projeto ativo; clique no projeto → foca último painel ativo.

**Diálogo de reposicionamento com projeto minimizado** (ver `ide-ux.md §3.4`):

- Interceptar `MOVE_PANEL` no reducer: se `panel.projectId` aponta para projeto minimizado → emitir evento que abre diálogo de confirmação antes de continuar.
- Implementar `ConfirmDialog` genérico reutilizável.

### 5.2 Panel Manager

Criar `ui/src/components/sidebar/PanelManager.tsx`:

- Painel especial (abrível via Dock) que lista todos os paineis em `IDEState.panels`.
- Agrupados por workspace.
- `[●]/[○]` toggle de visibilidade.
- Drag-and-drop interno: arrastar item de painel para outro grupo de workspace → `dispatch({ type: 'MOVE_PANEL_TO_WORKSPACE', panelId, targetWorkspaceId })`.
- Botão `×` por item → fechar painel.

**Arquivos a criar:**
```
ui/src/components/sidebar/
  ProjectSidebar.tsx
  PanelManager.tsx
ui/src/components/common/
  ConfirmDialog.tsx
```

**Arquivos a modificar:**
- `ui/src/store/ideStore.ts` — actions: `MINIMIZE_PROJECT`, `RESTORE_PROJECT`, `TOGGLE_PANEL_VISIBLE`, `MOVE_PANEL_TO_WORKSPACE`
- `ui/src/types/layout.ts` — adicionar `Project` e `IDEState.projects`

**Critério de conclusão:** sidebar de projetos com minimizar/restaurar funcionando. Panel Manager listando todos os paineis com toggle de visibilidade e drag entre workspaces.

---

## Fase 6 — Janela de Configurações (Geral + Temas)

> **Objetivo:** modal de configurações com seções Geral e Temas com preview ao vivo.

### 6.1 Componente `SettingsModal`

Criar `ui/src/components/settings/SettingsModal.tsx`:

- Modal overlay: fundo `rgba(0,0,0,0.6)`, conteúdo centralizado `80vw × 70vh` máx.
- Fecha com `Esc` ou clique no backdrop.
- Layout: nav list (200px) + conteúdo rolável.
- Nav list com indicador mint à esquerda no item ativo.
- Seções: Geral, Temas, Atalhos, Agentes, Providers, Avançado.

### 6.2 Seção Geral

Criar `ui/src/components/settings/sections/GeneralSection.tsx`:

- Campos descritos em `ide-ux.md §8.3`.
- Persist via `localStorage` (fase 9 migra para arquivo).
- Aplicar imediatamente: mudança de fonte → atualizar variável CSS `--font-family`; posição do Dock → `dispatch({ type: 'SET_DOCK_POSITION', position })`.

### 6.3 Sistema de Temas

Criar `ui/src/themes/`:

```
ui/src/themes/
  index.ts          # lista de temas nativos
  apply.ts          # aplica variáveis CSS ao :root
  types.ts          # interface Theme { id, label, variables }
  mint-dark.ts
  mint-light.ts
  monochrome.ts
  ocean.ts
  sunset.ts
```

`apply.ts`:
```ts
export function applyTheme(theme: Theme) {
  const root = document.documentElement
  for (const [key, value] of Object.entries(theme.variables)) {
    root.style.setProperty(key, value)
  }
}
```

### 6.4 Seção Temas

Criar `ui/src/components/settings/sections/ThemeSection.tsx`:

- Grade de cards 3 colunas.
- Clicar num card → `applyTheme(theme)` imediatamente (sem salvar, só preview).
- Card ativo com borda mint `2px` e badge "● ativo".
- Último card: "Explorar no Marketplace" → fecha settings, abre painel Marketplace filtrado por `category=theme`.

### 6.5 Thumbnail de Tema

Criar `ui/src/components/settings/ThemeThumbnail.tsx`:

- SVG inline (não imagem estática) de ~200×130px.
- Elementos renderizados com as cores do tema via props (não variáveis CSS do `:root`, pois o tema pode ainda não estar aplicado).
- Estrutura do SVG conforme `ide-ux.md §8.4`: title bar, file tree, tab bar do editor, trecho de código, input bar, status bar.

```tsx
<ThemeThumbnail theme={theme} />
// renderiza SVG com theme.variables['--bg'] como fundo, etc.
```

**Arquivos a criar:**
```
ui/src/components/settings/
  SettingsModal.tsx
  sections/
    GeneralSection.tsx
    ThemeSection.tsx
  ThemeThumbnail.tsx
ui/src/themes/
  index.ts
  apply.ts
  types.ts
  mint-dark.ts
  mint-light.ts
  monochrome.ts
  ocean.ts
  sunset.ts
```

**Arquivos a modificar:**
- `ui/src/store/ideStore.ts` — actions: `OPEN_SETTINGS`, `CLOSE_SETTINGS`, `SET_ACTIVE_THEME`, `SET_DOCK_POSITION`
- `ui/src/components/layout/Shell.tsx` — renderizar `<SettingsModal>` quando `state.settingsOpen`
- `ui/src/components/dock/DockItem.tsx` — item Configurações dispara `OPEN_SETTINGS`

**Critério de conclusão:** `Ctrl+,` abre modal. Seção Geral salva preferências. Seção Temas mostra grade com thumbnails SVG ao vivo; clicar num tema aplica instantaneamente.

---

## Fase 7 — Configurações Avançadas

> **Objetivo:** seções restantes da janela de configurações.

### 7.1 Seção Atalhos de Teclado

Criar `ui/src/components/settings/sections/KeybindingsSection.tsx`:

- Lista de ações agrupadas por contexto (Global, Editor, Terminal, Paineis).
- Campo de busca por nome ou tecla.
- Clicar num atalho → modo captura: próxima tecla pressionada define o novo atalho.
- Detecção de conflito: se combinação já usada por outra ação, exibir badge vermelho + nome da ação conflitante.
- Salvar em `localStorage` (chave `polvo.keybindings`).
- Hook `useKeybindings.ts` lê os atalhos e registra `keydown` listeners globais.

### 7.2 Seção Agentes

Criar `ui/src/components/settings/sections/AgentsSection.tsx`:

- `GET /api/config` para carregar `polvo.yaml`.
- Exibir lista de agentes com campos editáveis (modelo, guide, watch paths).
- `POST /api/config` para salvar ao clicar em "Salvar".
- Botão "Abrir polvo.yaml no editor" → dispatch para abrir o arquivo no painel Editor.

### 7.3 Seção Providers

Criar `ui/src/components/settings/sections/ProvidersSection.tsx`:

- `GET /api/providers` para listar providers e status.
- Indicador verde/vermelho por provider.
- Campos de API key mascarados (`type="password"`).
- Botão "Testar conexão" por provider → `POST /api/providers/test` (novo endpoint no Core).
- Salvar keys via `POST /api/config` (ou endpoint dedicado).

### 7.4 Seção Avançado

Criar `ui/src/components/settings/sections/AdvancedSection.tsx`:

- Campos descritos em `ide-ux.md §8.8`.
- "Exportar configurações" → gera JSON com `IDEState` serializável + config do Core → `download`.
- "Importar configurações" → file input → parse JSON → `dispatch({ type: 'IMPORT_STATE', state })`.
- "Resetar para padrões" → `ConfirmDialog` + `dispatch({ type: 'RESET_STATE' })`.

**Arquivos a criar:**
```
ui/src/components/settings/sections/
  KeybindingsSection.tsx
  AgentsSection.tsx
  ProvidersSection.tsx
  AdvancedSection.tsx
ui/src/hooks/
  useKeybindings.ts
```

**Critério de conclusão:** todas as seções de configurações funcionais. Atalhos customizáveis com detecção de conflito. Agentes e Providers editáveis e com feedback de teste.

---

## Fase 8 — Marketplace

> **Objetivo:** UI de descoberta e instalação de extensões; temas de marketplace aparecem na seção Temas com thumbnail automático.

### 8.1 Painel Marketplace

Criar `ui/src/components/marketplace/MarketplacePanel.tsx`:

- Layout: barra de busca + filtros de categoria (pills) + grade de cards de extensões.
- Dados: inicialmente mock local (`ui/src/marketplace/registry.ts`) com extensões de exemplo; fase futura conecta a API remota.
- Card de extensão: nome, descrição, categoria, versão, botão "Instalar" / "Instalado" / "Atualizar".

### 8.2 Instalação de Temas

Criar `ui/src/marketplace/installer.ts`:

- `installTheme(manifest)` → valida estrutura YAML/JSON do manifesto → registra o tema em `localStorage` (`polvo.installed-themes`) → chama `themeRegistry.add(theme)`.
- `ThemeSection` observa `themeRegistry` e re-renderiza os cards ao instalar novos temas.
- Thumbnails de temas instalados via marketplace usam o mesmo `ThemeThumbnail` com as variáveis do manifesto — nenhum asset adicional necessário.

### 8.3 Desinstalação de Temas

- Card de tema instalado: hover mostra botão `×` no canto superior direito.
- Clique → `ConfirmDialog` → `installer.uninstallTheme(id)` → remove de `localStorage` → `themeRegistry.remove(id)`.
- Se o tema desinstalado for o ativo, reverter para "Mint Dark".

### 8.4 Extensões de CLI Detector

- Extensão com `cli_detector: true` no manifesto pode declarar CLIs adicionais.
- `installer.installExtension` verifica o campo e atualiza a lista conhecida em `useCLIs`.

**Arquivos a criar:**
```
ui/src/components/marketplace/
  MarketplacePanel.tsx
  ExtensionCard.tsx
ui/src/marketplace/
  registry.ts       # lista de extensões disponíveis (mock inicial)
  installer.ts      # install/uninstall
  themeRegistry.ts  # registro dinâmico de temas
```

**Critério de conclusão:** Marketplace abre como painel. Temas podem ser "instalados" (mock) e aparecem imediatamente na seção Temas com preview SVG correto. Filtro por categoria funciona.

---

## Fase 9 — Persistência de Layout

> **Objetivo:** salvar e restaurar o layout completo entre sessões.

### 9.1 Serialização do Estado

- `IDEState` é serializável por design (plain objects, sem funções).
- Criar `ui/src/store/persistence.ts`:
  - `saveState(state: IDEState)` → `localStorage.setItem('polvo.layout', JSON.stringify(state))`
  - `loadState(): IDEState | null` → parse + validação básica de schema
- Salvar automaticamente no `useEffect` sempre que `state` mudar (debounce 500ms para não sobreescrever a cada keystroke).

### 9.2 Restauração na Inicialização

- `IDEProvider` tenta `loadState()` no mount.
- Se encontrar estado válido e config "Iniciar com último layout" ativa → usar como estado inicial.
- Caso contrário → `createInitialState()` (layout padrão: Arquivos + Terminal, Fase 1.5).

### 9.3 Migração de Schema

- Versionar o estado: `IDEState.version: number`.
- `loadState()` compara versão salva com versão atual — se diferente, descarta e usa estado inicial (não tentar migrar na fase 9; reservar para versões futuras).

### 9.4 Persistência no Core (futuro)

> Não implementar nesta fase. Registrar como decisão em aberto: mover persistência de `localStorage` para arquivo `.polvo/layout.json` no Core, servido via `GET /api/layout` e `POST /api/layout`.

**Arquivos a criar:**
```
ui/src/store/
  persistence.ts
```

**Arquivos a modificar:**
- `ui/src/store/IDEContext.tsx` — carregar estado persistido no mount, salvar no unmount/change
- `ui/src/components/settings/sections/GeneralSection.tsx` — toggle "Iniciar com último layout" lê/escreve flag de persistência

**Critério de conclusão:** fechar e reabrir a IDE restaura exatamente o layout anterior — workspaces, paineis, proporções de split e tema ativo.

---

## Dependências entre Fases

```
Fase 0 (tipos + tokens)
  └─ Fase 1 (split engine)
       └─ Fase 2 (dock)
       │    └─ Fase 4 (DnD do dock)
       └─ Fase 3 (workspaces)
            └─ Fase 4 (DnD cross-container)
                 └─ Fase 5 (sidebars)
Fase 6 (settings modal + temas)
  └─ Fase 7 (settings avançadas)
  └─ Fase 8 (marketplace)
       └─ temas de marketplace → Fase 6 já deve existir
Fase 9 (persistência)
  └─ depende de Fase 3 (workspaces estabilizados) e Fase 6 (toggle no Geral)
```

Fases 0–5 e Fases 6–9 podem correr em paralelo por times diferentes, pois as fases 6+ não dependem das fases 1–5 (só de Fase 0).

---

## Checklist por Fase

### Fase 0
- [ ] Tokens CSS em variáveis no `index.css`
- [ ] `ui/src/types/layout.ts` com todos os tipos
- [ ] `ideStore.ts` com reducer e `createInitialState()`
- [ ] `IDEContext.tsx` com Provider e hooks
- [ ] Build passa sem regressão

### Fase 1
- [ ] `SplitPane.tsx` com ratio em %
- [ ] `Splitter.tsx` com drag e visual correto
- [ ] `PanelFrame.tsx` com barra de título e indicador de foco
- [ ] `LayoutRenderer.tsx` recursivo
- [ ] Layout inicial: Arquivos 25% + Terminal 75%
- [ ] Splitter arrastável funciona

### Fase 2
- [ ] `Dock.tsx` com grupos e separadores
- [ ] `DockItem.tsx` com hover e tooltip
- [ ] `GET /api/clis` no Core retornando CLIs detectados
- [ ] `useCLIs.ts` consumindo o endpoint
- [ ] Clique no Dock abre painel correto
- [ ] Clique em CLI do Dock abre Terminal com comando

### Fase 3
- [ ] `WorkspaceTabBar.tsx` com criar/renomear/fechar/reordenar
- [ ] Troca de workspace preserva layout independente
- [ ] Workspace inicial criado com layout padrão

### Fase 4
- [ ] `DragContext.tsx` com estado global de drag
- [ ] Drop zones em `PanelFrame` com overlay visual
- [ ] Reducer `MOVE_PANEL` com split correto por zone
- [ ] Drag entre workspaces via hover em aba (600ms)
- [ ] DockItem draggável criando painel via drop

### Fase 5
- [ ] `ProjectSidebar.tsx` com lista, ativo e minimizar
- [ ] Bidireccionalidade foco painel ↔ projeto ativo
- [ ] `ConfirmDialog.tsx` genérico
- [ ] Diálogo ao reposicionar painel de projeto minimizado
- [ ] `PanelManager.tsx` listando todos os paineis
- [ ] Toggle visibilidade funciona
- [ ] Drag entre workspaces no PanelManager

### Fase 6
- [ ] `SettingsModal.tsx` abre/fecha com Esc e clique fora
- [ ] `Ctrl+,` abre configurações
- [ ] Seção Geral com todos os campos salvando em localStorage
- [ ] `ui/src/themes/` com 5 temas nativos
- [ ] `applyTheme()` funcionando
- [ ] `ThemeSection.tsx` com grade de cards
- [ ] `ThemeThumbnail.tsx` SVG dinâmico usando variáveis do tema
- [ ] Clicar num tema aplica instantaneamente

### Fase 7
- [ ] `KeybindingsSection.tsx` com captura de tecla
- [ ] Detecção de conflito em tempo real
- [ ] `useKeybindings.ts` registrando listeners globais
- [ ] `AgentsSection.tsx` lendo/escrevendo `polvo.yaml`
- [ ] `ProvidersSection.tsx` com status e botão testar
- [ ] `AdvancedSection.tsx` com exportar/importar/resetar

### Fase 8
- [ ] `MarketplacePanel.tsx` com busca e filtros
- [ ] `installer.ts` com install/uninstall
- [ ] `themeRegistry.ts` dinâmico
- [ ] Tema instalado aparece na seção Temas com thumbnail correto
- [ ] Desinstalação com fallback para Mint Dark

### Fase 9
- [ ] `persistence.ts` com save/load e versão de schema
- [ ] Restauração automática no mount
- [ ] Toggle "Iniciar com último layout" respeitado
- [ ] Descarte limpo de schema desatualizado
