# Documentação do Agente Polvo

Este documento detalha a implementação, arquitetura e funcionalidades do agente no projeto Polvo, baseando-se nos planos de design e na implementação atual em `app/internal/agent`.

---

## 1. Visão Geral

O Agente Polvo é o núcleo de execução inteligente do sistema. Ele opera em um ciclo iterativo (Loop) de **Prompt → LLM → Tools → LLM**, permitindo que modelos de linguagem interajam com o sistema de arquivos, executem comandos shell e gerenciem memória para resolver tarefas complexas de engenharia de software.

## 2. Arquitetura do Loop

A estrutura principal reside no pacote `agent.Loop`, que gerencia o ciclo de vida de uma tarefa.

### 2.1 Modos de Operação

O agente suporta dois fluxos principais de execução:

1.  **Single-phase (Padrão):** O agente recebe uma tarefa e utiliza todas as ferramentas disponíveis para resolvê-la diretamente.
2.  **Two-phase (Architect/Editor):**
    *   **Phase 1 (Architect):** Um modelo focado em raciocínio usa ferramentas de leitura para analisar o problema e produz um `WorkPlan`.
    *   **Phase 2 (Editor):** Um modelo focado em edição (muitas vezes mais rápido/barato) recebe o plano e utiliza ferramentas de escrita para aplicar as mudanças.
    *   *Benefício:* Melhora a precisão em tarefas complexas e reduz custos ao usar modelos menores para a fase de edição.

## 3. Comportamentos Inteligentes

O Polvo implementa mecanismos avançados para garantir que o agente não entre em loops infinitos e que o resultado final seja de alta qualidade.

### 3.1 Detecção de Travamento (Stuck Detection)
O `StuckDetector` monitora 5 padrões específicos para abortar execuções ineficientes:
*   **Repeat:** Mesma ferramenta + input repetidos N vezes.
*   **Error Loop:** Mesma ferramenta falhando consecutivamente.
*   **Cyclic:** Padrão A → B → A → B detectado no histórico.
*   **Monologue:** Múltiplos turnos de texto sem chamada de ferramentas.
*   **Context Window:** Sucessivas condensações de contexto sem progresso real.

### 3.2 Reflexão e Validação (Reflection Loop)
Após a conclusão de uma tarefa, o agente pode passar por fases de reflexão:
*   **Phases:** Executa comandos (ex: `lint`, `test`) para validar o trabalho.
*   **Feedback Loop:** Se uma validação falha, o erro é alimentado de volta ao agente como uma nova instrução, permitindo que ele se auto-corrija até N vezes.

## 4. Gerenciamento de Contexto

Para lidar com limites de tokens, o Polvo utiliza uma estratégia de cascata (Context Fallback):

1.  **LLM Summarization:** Usa um modelo mais barato para resumir as partes mais antigas da conversa, mantendo o final intacto.
2.  **Observation Masking:** Substitui outputs de ferramentas antigos por placeholders (ex: `[tool output omitted - 500 chars]`).
3.  **Amortized Condenser:** Descarta o meio do histórico (sem LLM) quando ultrapassa `MaxSize` mensagens — O(1) em tokens.
4.  **Sliding Window / Pruning:** Truncagem inteligente das mensagens mais antigas como último recurso.

### Session Tasks & Questions (Roadmap)

`/task <prompt>` e `/question <prompt>` — comandos planejados (plano 52) que resetam o contexto agêntico intencionalmente, agrupam as mensagens de cada unidade de trabalho e geram um summary automático em background ao término. O usuário pode referenciar tasks anteriores com `@@task[task#02]` no próximo prompt — o sistema aguarda o summary e injeta o contexto inline antes de enviar ao modelo.

## 5. Segurança e Autonomia

O sistema de permissões garante que o agente opere dentro de limites seguros.

### 5.1 Modos de Autonomia
*   **Full:** Executa ferramentas e aplica mudanças sem confirmação.
*   **Supervised:** Requer aprovação humana para ferramentas sensíveis (escrita, bash).
*   **Plan:** Modo de apenas leitura (read-only).

### 5.2 Approval Gates & Audit Log
*   **PermissionCallback:** Permite integrar o agente com interfaces TUI ou Web para solicitar aprovação em tempo real.
*   **AuditLogger:** Registra cada chamada de ferramenta, input, decisão e duração para fins de auditoria e depuração.
*   **Patch Sandbox:** Em modo supervisionado, as edições de arquivos são acumuladas em um sandbox de memória e só aplicadas ao disco após aprovação explícita.

## 6. Funcionalidades de Ferramentas (Toolbox)

### 6.1 Edit Robusto (5-Level Fallback)
A ferramenta `edit` utiliza uma cascata de 5 níveis para garantir que as edições sejam aplicadas mesmo com pequenas divergências de formatação:
1.  **Exact Match:** Busca a string exata.
2.  **Whitespace-Flexible:** Ignora variações de espaços em branco e quebras de linha.
3.  **Relative Indent:** Ignora diferenças de indentação global, focando na estrutura do código.
4.  **Fuzzy Match (Levenshtein):** Utiliza distância de edição para encontrar o bloco mais provável (com threshold de segurança).
5.  **Whole-file Rewrite:** Fallback para reescrita completa do arquivo quando mudanças pontuais falham.

### 6.2 Bash Persistent Sessions
Diferente de execuções efêmeras, o Polvo mantém uma sessão Bash persistente:
*   **Estado Preservado:** Comandos como `cd`, `export` e definições de funções persistem entre diferentes chamadas de ferramentas.
*   **Interactive Detection:** Bloqueia automaticamente comandos que requerem TTY interativo (ex: `vim`, `nano`, `ssh`, `sudo`) para evitar que o agente trave.
*   **Sentinel Protocol:** Utiliza marcadores de saída (EOF markers) para capturar o código de saída e o output de forma confiável.

### 6.3 Repository Mapping (RepoMap)
Gera uma visão condensada do repositório para o LLM:
*   **Symbol Extraction:** Extrai funções, classes e tipos exportados (suporta Go, TS/JS, Python).
*   **Token Budgeting:** Garante que o mapa do repositório caiba dentro de um limite de tokens (ex: 2000 tokens).
*   **Focus Boosting:** Aumenta a pontuação de arquivos relevantes para a tarefa atual para priorizá-los no mapa.
*   **Build Cache (TTL):** Resultado em memória com TTL de 30s para evitar recomputação entre turnos da mesma sessão.

### 6.4 Indexação BM25 (ChunkIndex)
Paralelamente ao repo map de símbolos, o `ChunkIndex` mantém um índice SQLite com FTS5 (BM25) sobre chunks do código fonte, usado pela tool `search_code`:
*   **Indexação no startup:** `Indexer.IndexAll()` é chamado em background goroutine na inicialização — incremental por hash de arquivo.
*   **Atualização via watcher:** Quando um arquivo muda, `Indexer.IndexFile(path)` atualiza os chunks afetados em goroutine separada.
*   **Limpeza ao deletar:** `ChunkIndex.DeleteByPath(path)` remove chunks de arquivos deletados.
*   **Verificação via /doctor:** `/doctor` verifica se o índice está populado e oferece reindex automático como fix.

## 7. Funcionalidades Avançadas

### 7.1 Microagentes
Permitem a injeção dinâmica de contexto e instruções baseadas em gatilhos (triggers) como palavras-chave, extensões de arquivos ou regex. Isso permite especializar o comportamento do agente sem sobrecarregar o prompt de sistema global.

### 7.2 Checkpoint & Restore
O sistema registra um log de eventos que permite:
*   **Time-travel:** Voltar o estado do agente para um ponto anterior.
*   **Restore:** Recuperar a sessão após uma falha ou interrupção.

### 7.3 Gerenciamento de Memória
*   **Semantic Search:** Busca por relevância em memórias passadas.
*   **TTL (Time-To-Live):** Expiração automática de memórias irrelevantes.
*   **Recall:** Recuperação de contexto entre diferentes sessões.

## 8. Observabilidade e Métricas

Cada execução do agente gera um `AgentMetrics` contendo:
*   Contagem de turnos e tokens utilizados.
*   Custo real em USD (baseado na tabela de preços do modelo).
*   Contagem de chamadas por ferramenta.
*   Pressão da janela de contexto (Context Window Pressure).
*   Detecções de travamento e tentativas de reflexão.

---
*Documentação gerada automaticamente com base na implementação v1.0.*
