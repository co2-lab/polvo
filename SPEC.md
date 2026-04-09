# Polvo - Orquestrador de Agentes Guiados para Projetos

## O que é

Ferramenta em Go que monitora um repositório Git e, ao detectar alterações em arquivos específicos, dispara agentes de IA especializados para gerar, verificar e corrigir artefatos do projeto. Cada agente é orientado por um **guide** — um documento de referência que define as regras e padrões para aquele domínio.

Toda alteração gerada por um agente é entregue via **Pull Request**. Outro agente (o reviewer) revisa o PR usando seus próprios guides. Se o PR é rejeitado, o agente autor corrige e abre novo PR. O Git é o backbone de todo o fluxo — nada é alterado diretamente na branch principal.

O Polvo vem com um **IDE web embutido** — editor de código, terminal integrado e dashboard de agentes — servido pelo próprio binário em `http://localhost:7373`. Pode ser usado no browser ou como app desktop via Tauri.

---

## Conceitos Fundamentais

### Guide
Documento de referência (markdown, yaml ou outro formato texto) que define as regras e padrões que um agente deve seguir. O guide é a "fonte de verdade" do agente — serve como contexto e critério para o agente vinculado.

#### Sistema de Camadas

O Polvo opera com duas camadas de configuração:

**Camada base (built-in)** — o Polvo vem com um conjunto de guides, prompts e configurações embutidos no binário. São defaults sensatos que funcionam out-of-the-box. O usuário não precisa configurar nada para começar.

**Camada do projeto (override)** — o usuário pode estender ou sobrescrever a camada base no seu projeto. Para cada guide, o usuário escolhe o comportamento:
- **extend** (padrão) — o guide do projeto complementa o guide base. As regras base continuam valendo e o usuário adiciona regras específicas. O agente recebe ambos os conteúdos (base + projeto) concatenados.
- **replace** — o guide do projeto substitui completamente o guide base.

Configurado por guide no `polvo.yaml` via campo `mode: extend | replace`.

Se existe guide do projeto, aplica o mode configurado. Se não existe, usa o guide base embutido.

#### Guides Base (Built-in)

Os 7 guides que vêm embutidos no Polvo:

- **spec** — funciona como **template**: define a estrutura, formato e critérios de qualidade que uma spec deve ter (seções obrigatórias, nível de detalhe, como descrever requisitos). Não contém regras de negócio — essas são específicas de cada projeto. O agente de spec usa esse template para gerar e validar os `*.spec.md`.

- **features** — define como funcionalidades devem ser descritas em formato Gherkin (Given/When/Then). O agente vinculado gera e mantém os cenários a partir da spec, garantindo que estejam completos, consistentes e alinhados.

- **tests** — define as diretrizes de testes do projeto (cobertura mínima, padrões de nomeação, tipos de teste esperados). O agente vinculado gera e verifica os testes a partir dos cenários Gherkin.

- **lint** — define regras de estilo e formatação do código (além do que linters tradicionais cobrem). Atua exclusivamente como **gate de review** em PRs — não gera código, apenas aprova ou rejeita.

- **best-practices** — define boas práticas específicas do projeto (patterns a seguir, anti-patterns a evitar, convenções de arquitetura). Atua exclusivamente como **gate de review** em PRs — não gera código, apenas aprova ou rejeita.

- **review** — define os critérios gerais de review de PRs: coerência entre artefatos, qualidade do código, aderência à spec, completude. É o guide do agente reviewer que coordena o processo de aprovação dos PRs, orquestrando os gates de lint e best-practices.

- **docs** — sintetizador de documentação. Lê os artefatos gerenciados pelos outros guides (specs, features) e gera/atualiza a documentação do projeto. Disparado quando specs ou features são alterados.

Cada guide base vem com seu prompt template embutido. O usuário não precisa escrever prompts para usá-los.

#### Guides Customizados

Além dos 7 guides base, o usuário pode criar guides novos para necessidades específicas:
- Guide de segurança para projetos com dados sensíveis
- Guide de performance para projetos com SLAs de latência
- Guide de acessibilidade para projetos frontend
- Guide de migração para projetos com regras específicas de banco

Guides customizados podem atuar como geradores (criam artefatos via PR) ou como gates de review (aprovam/rejeitam PRs). Configurado no `polvo.yaml`.

#### Onde ficam os guides

Os guides do projeto ficam em `guides/` e são versionados junto com o código. Os guides base ficam embutidos no binário — o usuário só cria arquivos quando quer sobrescrever/estender ou quando cria guides customizados.

### Agente
Unidade de execução vinculada a um guide. Existem dois papéis:

- **Agente autor** — gera ou altera artefatos. Cria uma branch, faz as alterações, abre um PR. Exemplos: agente de spec, features, tests, docs.
- **Agente reviewer** — revisa PRs abertos por agentes autores. Usa seus guides como critério de aprovação. Exemplos: agente de review (coordenador), agente de lint (gate), agente de best-practices (gate).

Tipos de implementação:
- **llm** — alimentado por um modelo de IA via provider configurável. É o tipo principal.
- **external** — binário ou script externo. Útil para integrar ferramentas existentes (linters, formatters, scanners).
- **builtin** — plugin Go interno.

### Provider de IA
Camada de abstração que desacopla os agentes LLM dos modelos e APIs específicos. Cada agente LLM declara qual provider e modelo quer usar.

Providers suportados:
- **Claude** (Anthropic)
- **Gemini** (Google)
- **OpenAI** (GPT)
- **Ollama** (local — CodeLlama, Mistral, Llama, etc.)
- **OpenAI-compatible** (qualquer endpoint customizado)

Isso permite que gates de lint rodem num modelo local rápido e barato (Ollama), enquanto o agente de spec rode num modelo mais capaz (Claude Opus). O usuário escolhe a relação custo/qualidade/velocidade por agente.

### Laudo de Análise
Documento estruturado gerado por agentes, usado como:
- Comentário em PRs (motivo de aprovação ou rejeição)
- Registro de conflitos spec vs. interface para decisão humana
- Histórico de análises por arquivo

Cada laudo contém:
- Identificação do agente e guide que gerou, e do arquivo analisado
- Timestamp e severidade (info, warning, high, critical)
- Lista de achados com localização, tipo, mensagem e sugestão
- Contexto: diff, arquivos relacionados

Laudos são persistidos em `.polvo/reports/` e anexados como comentários nos PRs.

### Arquivo de Interface
Qualquer arquivo do projeto que representa um ponto de contato entre o sistema e seus usuários — rotas de API, handlers, controllers, CLI commands, componentes de UI, schemas, contratos, eventos, etc. Definido no `polvo.yaml`, não por convenção fixa.

### Arquivos Derivados
Cada arquivo de interface tem artefatos derivados co-localizados — na mesma pasta, com o mesmo nome base:

```
screens/
  HomeScreen.tsx          # arquivo de interface
  HomeScreen.spec.md      # spec
  HomeScreen.feature      # cenários Gherkin
  HomeScreen.test.tsx     # testes
```

Convenção configurável no `polvo.yaml` via variáveis `{{dir}}`, `{{name}}` e `{{ext}}`. Padrão: co-located.

---

## Filosofia: Spec-First

O Polvo opera com a filosofia **spec-first** — a spec é a fonte de verdade do projeto.

### Fluxo principal (spec → código)
1. O usuário escreve ou altera a spec (`*.spec.md`)
2. O agente de interface gera/altera o código de interface e abre um **PR**
3. O agente de review coordena a revisão do PR:
   - Agente de lint verifica aderência ao guide de lint → aprova ou rejeita
   - Agente de best-practices verifica aderência ao guide → aprova ou rejeita
   - Agente de review verifica coerência geral → aprova ou rejeita
4. Se rejeitado: o agente autor recebe o feedback, corrige e abre **novo PR**
5. Se aprovado: o PR é mergeado
6. O merge dispara a próxima etapa da cadeia (features, tests, docs)

### Fluxo inverso (código → conflito)
Quando o usuário altera um arquivo de interface diretamente (sem alterar a spec antes):
1. O agente de spec verifica coerência entre a interface e a spec
2. Se coerente: nenhuma ação
3. Se divergente: gera um **laudo de conflito** e pergunta ao usuário:
   - **Atualizar a spec** — a interface mudou intencionalmente, a spec deve refletir a nova realidade
   - **Corrigir a interface** — a alteração foi incoerente, reverter para aderir à spec

O Polvo nunca resolve conflitos spec vs. interface automaticamente. A decisão é sempre do usuário.

### Fluxo de criação inicial
Quando o `*.spec.md` não existe para um arquivo de interface:
- O agente de spec gera um **scaffold** de spec baseado no código existente e no guide de spec
- O scaffold é aberto como PR para o usuário revisar, completar e aprovar
- Somente após a spec ser aprovada pelo usuário, a cadeia pode avançar

---

## Cadeia de Reação

Toda alteração gera um PR. Toda revisão é feita por agentes reviewers. A cadeia só avança quando o PR é aprovado e mergeado.

```
*.spec.md alterado (pelo usuário)
  └─→ agente interface       → abre PR com código gerado/alterado
        └─→ review (lint + best-practices + review)
              ├─ aprovado    → merge → dispara agente features
              └─ rejeitado   → agente corrige → novo PR

*.feature aprovado e mergeado
  └─→ agente tests           → abre PR com testes gerados/alterados
        └─→ review
              ├─ aprovado    → merge → dispara agente docs
              └─ rejeitado   → agente corrige → novo PR

*.spec.md ou *.feature alterado
  └─→ agente docs            → abre PR com documentação atualizada
        └─→ review
              ├─ aprovado    → merge
              └─ rejeitado   → agente corrige → novo PR
```

**Fluxo inverso (usuário altera interface):**
```
Arquivo de interface alterado (pelo usuário)
  ├─→ agente spec            → verifica coerência com *.spec.md
  │     ├─ coerente          → OK
  │     └─ divergente        → laudo de conflito → pergunta ao usuário
  ├─→ agente lint            → verifica aderência (gera laudo se violação)
  └─→ agente best-practices  → verifica aderência (gera laudo se violação)
```

**Auto-verificação de testes:**
```
*.test.* alterado (por humano ou por merge de PR)
  └─→ agente tests           → verifica coerência (duplicatas, inconsistências)
        └─ se problemas      → abre PR com correções → review
```

### Proteção contra loops
- PRs gerados por agentes são identificados (label, prefixo de branch)
- Um PR rejeitado pode gerar no máximo N tentativas de correção (configurável, default: 3)
- Após N rejeições, o Polvo escala para decisão humana
- A cadeia só avança em merges na branch principal, nunca em branches de PR

### Processo de Review

O review de PRs é coordenado pelo agente de review, que opera em etapas:

1. **Gates automáticos** — agente de lint e agente de best-practices analisam o diff do PR. Cada um usa seu guide como critério. Se qualquer gate rejeita, o PR é rejeitado com o laudo como comentário.
2. **Review geral** — se os gates passam, o agente de review analisa o PR como um todo: coerência entre artefatos, aderência à spec, qualidade geral. Usa o guide de review como critério.
3. **Decisão** — aprovação (merge automático) ou rejeição (comentário com feedback para o agente autor).

Gates customizados podem ser adicionados via guides customizados com papel de reviewer.

### Propagação de Guides
Quando um guide é alterado (ex: o guide de lint é atualizado com novas regras), o Polvo re-dispara os agentes vinculados contra todos os arquivos monitorados para verificar conformidade com as novas regras. Violações são tratadas como novos achados e geram PRs de correção.

---

## Funcionalidades

### Monitoramento de Arquivos
O Polvo observa o repositório Git em tempo real. O monitoramento é configurado por padrões glob, e cada padrão é associado aos agentes que devem ser disparados.

Detecta criação, modificação e remoção de arquivos. Debounce configurável evita disparos duplicados em salvamentos rápidos.

### Configuração do Projeto (`polvo.yaml`)
Arquivo do projeto que estende a configuração base do Polvo. O usuário só precisa declarar o que quer mudar — todo o resto usa os defaults embutidos.

- **project** — nome e diretório raiz
- **providers** — configuração dos providers de IA (tipo, API key via env var, modelo default, endpoint). Obrigatório: pelo menos um provider.
- **interfaces** — define quais arquivos são de interface (padrões glob e/ou listagem explícita) e a convenção de nomeação dos derivados.
- **guides** — sobrescreve ou estende guides base, e define guides customizados. Para cada guide:
  - `mode: extend | replace` — complementa ou substitui o guide base (default: extend)
  - `file` — caminho do arquivo de guide do projeto
  - `provider` / `model` — override do provider/modelo para o agente
  - `prompt` — override do prompt template
  - `role: author | reviewer` — se o guide é de um agente autor ou reviewer
- **chain** — customização da cadeia de reação (quais agentes disparam em cada etapa, ordem). Estende a cadeia padrão.
- **review** — configuração do processo de review:
  - `gates` — quais guides atuam como gates de PR (default: lint, best-practices)
  - `max_retries` — tentativas máximas de correção antes de escalar para humano (default: 3)
  - `auto_merge` — se PRs aprovados são mergeados automaticamente (default: true)
- **git** — configuração de Git:
  - `branch_prefix` — prefixo para branches do Polvo (default: `polvo/`)
  - `pr_labels` — labels para PRs gerados (default: `["polvo", "automated"]`)
  - `target_branch` — branch alvo para PRs (default: `main`)
- **settings** — configurações globais (debounce, diretório de reports, log level, paralelismo máximo)

API keys são referenciadas via variáveis de ambiente, nunca hardcoded.

Exemplo de `polvo.yaml`:
```yaml
project:
  name: "meu-projeto"

providers:
  default:
    type: ollama
    base_url: "http://localhost:11434"
    default_model: "codellama:13b"
  claude:
    type: claude
    api_key: "${ANTHROPIC_API_KEY}"
    default_model: "claude-sonnet-4-6"

interfaces:
  patterns:
    - "screens/**/*.tsx"
    - "api/handlers/**/*.go"
  derived:
    spec:     "{{dir}}/{{name}}.spec.md"
    features: "{{dir}}/{{name}}.feature"
    tests:    "{{dir}}/{{name}}.test.{{ext}}"

guides:
  spec:
    provider: claude
    model: "claude-opus-4-6"
  review:
    provider: claude
    model: "claude-opus-4-6"

review:
  gates: [lint, best-practices]
  max_retries: 3
  auto_merge: true

git:
  branch_prefix: "polvo/"
  target_branch: "main"
```

### Execução de Agentes
Quando um arquivo é alterado:
1. O Polvo identifica quais agentes devem ser disparados (via cadeia de reação)
2. Para cada agente, renderiza o prompt template com o guide + arquivo + diff + contexto
3. Envia ao provider configurado
4. Parseia a resposta para o formato padronizado
5. **Agente autor:** cria branch `polvo/<guide>/<arquivo>`, commita as alterações, abre PR
6. **Agente reviewer:** analisa o diff do PR, posta comentários, aprova ou rejeita

Cada agente tem timeout configurável. O paralelismo máximo é configurável.

### Sistema de Laudos
Laudos são gerados em todas as etapas — tanto por agentes autores quanto reviewers. Servem como:
- Comentários em PRs (justificativa de aprovação/rejeição)
- Registro de conflitos para decisão humana
- Histórico por arquivo para análise de tendências

Persistidos em `.polvo/reports/` organizados por data.

### Prompt Templates
Templates usados por agentes LLM, com variáveis renderizadas pelo Polvo:
- `{{file}}` — caminho do arquivo alterado
- `{{content}}` — conteúdo completo do arquivo alterado
- `{{diff}}` — diff da alteração
- `{{guide}}` — conteúdo completo do guide vinculado ao agente
- `{{event}}` — tipo de evento (created, modified, deleted)
- `{{project_root}}` — raiz do projeto
- `{{previous_reports}}` — laudos de agentes anteriores no pipeline
- `{{file_history}}` — histórico de laudos deste arquivo
- `{{interface}}` — conteúdo do arquivo de interface relacionado
- `{{spec}}` — conteúdo do `*.spec.md` relacionado
- `{{feature}}` — conteúdo do `*.feature` relacionado
- `{{derived.*}}` — acesso a qualquer artefato derivado do mesmo arquivo de interface
- `{{pr_diff}}` — diff do PR (para agentes reviewers)
- `{{pr_comments}}` — comentários anteriores no PR (para retentativas)

Prompts podem ser inline no YAML ou em arquivos separados.

### IDE Web Embutido

O Polvo inclui um IDE web completo servido pelo binário em `http://localhost:7373`:

- **Editor de código** — Monaco Editor com syntax highlighting, múltiplas abas e diff inline
- **Terminal integrado** — PTY real via WebSocket (Unix: `creack/pty`, Windows: ConPTY)
- **Explorador de arquivos** — árvore de diretórios com operações CRUD
- **Dashboard de agentes** — status em tempo real dos agentes via SSE
- **Detecção de CLIs** — detecta automaticamente ferramentas instaladas (Claude, Copilot, Gemini, etc.)
- **Múltiplos projetos** — gerencia e alterna entre projetos abertos
- **App desktop** — wrapper Tauri com atualização automática

### API HTTP

O servidor expõe uma API REST em `/api/` consumida pelo IDE:

- `GET /api/status` — versão, config e status dos agentes
- `GET /api/providers` — lista providers configurados
- `GET /api/agents` — agentes em execução
- `GET /api/reports` — laudos gerados
- `GET/POST /api/config` — leitura e escrita do `polvo.yaml`
- `GET /api/projects`, `GET /api/projects/:id` — gerenciamento de projetos
- `GET /api/fs/list|read`, `POST /api/fs/write|delete|rename|mkdir` — API de filesystem
- `GET /api/clis` — detecção de CLIs de agentes instalados
- `POST /api/chat` — chat com agente LLM
- `GET /api/diff`, `POST /api/diff/accept|reject` — revisão de diffs
- `GET /api/doctor`, `POST /api/doctor/fix` — diagnóstico e correção automática
- `GET /events` — SSE stream de eventos em tempo real
- `WS /terminal/ws` — WebSocket PTY para terminal integrado

### CLI
- `polvo` — inicia o servidor (porta 7373 por padrão)
- `polvo init` — inicializa projeto criando `polvo.yaml` mínimo
- `polvo watch` — inicia monitoramento em tempo real
- `polvo run <arquivo>` — executa cadeia manualmente para um arquivo
- `polvo run --guide <guide>` — re-executa um guide contra os arquivos do projeto
- `polvo bootstrap` — setup inicial: escaneia interfaces, gera scaffolds de spec via PR para o usuário completar
- `polvo report [--file <arq>] [--guide <guide>]` — consulta laudos
- `polvo guides` — lista guides configurados com papel, provider e status
- `polvo providers` — lista providers e executa health check
- `polvo status` — status do monitoramento e PRs pendentes
- `polvo prs` — lista PRs abertos pelo Polvo com status de review
- `polvo history <arquivo>` — histórico de análises e PRs

### Protocolo de Agentes Externos
Agentes externos recebem contexto via variáveis de ambiente:
- `POLVO_FILE` — caminho do arquivo alterado
- `POLVO_GUIDE` — caminho do guide vinculado
- `POLVO_GUIDE_CONTENT` — conteúdo do guide
- `POLVO_EVENT` — tipo de evento
- `POLVO_DIFF` — diff da alteração
- `POLVO_PROJECT_ROOT` — raiz do projeto
- `POLVO_REPORT_DIR` — diretório de laudos
- `POLVO_PREVIOUS_REPORTS` — laudos anteriores no pipeline
- `POLVO_PR_DIFF` — diff do PR (para reviewers)
- `POLVO_PR_URL` — URL do PR (para reviewers)

Retorna JSON padronizado com status, alterações e laudo.

---

## Estrutura do Projeto

```
polvo/
  app/          # Go backend — servidor HTTP, orquestração de agentes
    cmd/
      polvo/    # Binário principal (servidor + IDE)
    internal/
      server/       # Servidor HTTP e rotas da API
      agent/        # Execução de agentes LLM
      pipeline/     # Cadeia de reação
      provider/     # Abstração de providers de IA
      guide/        # Sistema de guides (leitura e resolução)
      config/       # Parsing do polvo.yaml
      watcher/      # Monitoramento de arquivos com debounce
      gitclient/    # GitHub API
      clidetect/    # Detecção de CLIs instaladas
  ui/           # Frontend React (Vite + TypeScript)
  desktop/      # Wrapper Tauri (app desktop)
  embed/        # Guides, prompts e config base embutidos no binário
```

## Estrutura Interna

**Embutido no binário do Polvo (camada base):**
```
embed/
  guides/
    spec.md              # Guide base de spec (template)
    features.md          # Guide base de features (Gherkin)
    tests.md             # Guide base de testes
    lint.md              # Guide base de lint/estilo
    best-practices.md    # Guide base de boas práticas
    review.md            # Guide base de review de PRs
    docs.md              # Guide base de documentação
  prompts/
    spec.md              # Prompt do agente de spec
    features.md          # Prompt do agente de features
    tests.md             # Prompt do agente de testes
    lint.md              # Prompt do gate de lint
    best-practices.md    # Prompt do gate de best-practices
    review.md            # Prompt do agente reviewer
    docs.md              # Prompt do agente de docs
  config.yaml            # Configuração base (cadeia, settings)
```

**No projeto do usuário (camada do projeto):**
```
projeto/
  polvo.yaml             # Configuração do projeto (estende a base)
  guides/                # Opcional — só se quiser customizar
    lint.md              # Exemplo: regras de lint do projeto
    security.md          # Exemplo: guide customizado
  .polvo/
    reports/             # Laudos gerados, organizados por data
    history/             # Histórico consolidado por arquivo
    polvo.log            # Log de operações
```

---

## Requisitos Não-Funcionais

- **Performance** — watcher não impacta o filesystem; debounce evita excesso de disparos
- **Isolamento** — cada PR é uma branch isolada; agentes não alteram a branch principal diretamente
- **Extensibilidade** — novos guides, agentes e providers podem ser adicionados sem alterar o core
- **Resiliência** — falha de um agente não trava a cadeia; rejeição de PR gera retentativa controlada
- **Rastreabilidade** — toda alteração tem um PR com laudo, comentários e histórico no Git
- **Segurança** — API keys em variáveis de ambiente; agentes nunca fazem push direto para main
- **Idempotência** — mesmo agente no mesmo arquivo sem alteração não gera PR novo

---

## Fases de Implementação

### Fase 1 — Core
Estrutura do projeto Go, parser do `polvo.yaml`, file watcher com debounce, sistema de guides (leitura e vinculação com agentes), integração Git básica (branch, commit), CLI (`init`, `watch`, `run`, `bootstrap`).

### Fase 2 — Providers de IA
Interface de providers, implementações (Ollama, Claude, OpenAI, Gemini, OpenAI-compatible), prompt templates com variáveis, execução de agentes LLM, CLI `providers`.

### Fase 3 — PRs e Review
Criação de PRs via Git/GitHub API, agente reviewer com guide de review, gates de lint e best-practices como etapas do review, ciclo de rejeição → correção → novo PR, limite de retentativas com escalação para humano, CLI `prs`, `review`.

### Fase 4 — Cadeia Completa
Cadeia de reação completa (spec → interface → features → tests → docs), detecção de conflito spec vs. interface, fluxo de criação inicial (scaffold de spec), propagação de guides, CLI `report`, `guides`, `history`.

### Fase 5 — Refinamento
Rate limiting por provider, fallback de provider, cache de respostas LLM, métricas de tokens, modo CI/CD, modo batch, notificações para conflitos e PRs pendentes de review humano.
