# Documentação da IDE Polvo (UI)

Este documento detalha as funcionalidades, comportamentos e arquitetura da interface do usuário (UI) do projeto Polvo, localizada em `ui/`.

---

## 1. Visão Geral

A UI do Polvo é uma IDE (Integrated Development Environment) moderna construída com **React**, **TypeScript** e **Tailwind CSS**, rodando sobre o framework **Tauri**. Ela é projetada para ser altamente flexível, suportando múltiplos workspaces, painéis divisíveis (split panels) e uma integração profunda com agentes de IA.

## 2. Tecnologias Principais

*   **Framework:** React 19 (Vite) + Tauri 2.0.
*   **Estado:** Zustand (com persistência local).
*   **Editor:** Monaco Editor (`@monaco-editor/react`).
*   **Terminal:** xterm.js (`@xterm/xterm`).
*   **Layout:** `react-resizable-panels` e `react-arborist` (árvore de arquivos).
*   **Animações:** Framer Motion.
*   **Ícones:** Lucide React e Material Icon Theme.

## 3. Sistema de Layout e Workspaces

A IDE implementa um sistema de gerenciamento de janelas complexo e persistente.

### 3.1 Workspaces
O usuário pode criar múltiplos "Workspaces", cada um com seu próprio layout de painéis.
*   **Persistência:** O layout de cada workspace é salvo no `localStorage`.
*   **Gerenciamento:** Suporte para criar, renomear, duplicar e fixar (pin) workspaces.

### 3.2 Painéis Dinâmicos (Split Panels)
A IDE utiliza um sistema de árvore binária para gerenciar a divisão da tela.
*   **Split Horizontal/Vertical:** Usuários podem dividir qualquer painel em qualquer direção.
*   **Tabs:** Cada painel pode conter múltiplas abas (Tabs).
*   **Behaviors:** Diferentes tipos de conteúdo têm comportamentos de abertura distintos:
    *   **Grouped:** Abre em abas no painel de editores (ex: arquivos de código).
    *   **New Panel:** Abre em um novo painel lateral ou inferior (ex: terminais, chat).

### 3.3 Dock e Side Panels
*   **Dock:** Uma barra de ferramentas flutuante ou fixa que permite acesso rápido a Explorer, Terminal, Chat, Diff, etc.
*   **Customização:** O Dock pode ser posicionado em qualquer borda da tela (top, bottom, left, right).
*   **Side Panels:** Painéis laterais colapsáveis para navegação de arquivos e configurações.

## 4. Funcionalidades de Edição e Código

### 4.1 Monaco Editor
Integração completa com o motor do VS Code para edição de arquivos.
*   **Dirty State:** Rastreia arquivos modificados que ainda não foram salvos.
*   **Temas:** Suporte a múltiplos temas visuais (Midnight, Tokyo Night, Monokai, Cyberpunk, etc.).

### 4.2 Explorador de Arquivos (File Tree)
*   **Multi-projeto:** Suporte para visualizar múltiplos diretórios raiz simultaneamente.
*   **Drag & Drop:** (Implementado via `react-arborist`) para organização de arquivos.

### 4.3 Sessões de Diff
Sistema dedicado para comparação de arquivos e diretórios.
*   **Compare:** Seleção de arquivos "Lado Esquerdo" e "Lado Direito".
*   **Sessões:** Possibilidade de manter múltiplas sessões de comparação ativas.

## 5. Integração com Agentes e Sistema

### 5.1 Monitoramento via SSE (Server-Sent Events)
A UI mantém uma conexão em tempo real com o backend Go (sidecar do Tauri) para:
*   **Agent Status:** Monitorar em tempo real quais agentes estão rodando, em quais arquivos e seu progresso.
*   **Logs:** Visualizar o fluxo de logs do sistema centralizado.
*   **Snapshot:** Sincronizar o estado global (versão, projeto ativo, watchers).

### 5.2 Terminal (xterm.js)
Terminais integrados que suportam:
*   **Persistência de Sessão:** Sessões de terminal que sobrevivem a mudanças de layout dentro da mesma execução.
*   **CLI Detection:** Detecção automática de ferramentas de linha de comando disponíveis no sistema.

## 6. Configurações e Atalhos

### 6.1 Atalhos de Teclado (Shortcuts)
Sistema de atalhos customizáveis para:
*   **Quick Open (Ctrl+P)**
*   **Global Search (Ctrl+Shift+F)**
*   **Toggle Terminal (Ctrl+`)**
*   **Save (Ctrl+S)**

### 6.2 Configurações Gerais
*   Escolha de idioma (Português/Inglês).
*   Customização de fontes e tamanhos do editor.
*   Configuração de confirmação ao fechar o app (integrado com o ciclo de vida do Tauri).

## 7. Arquitetura de Estado (useIDEStore)

O estado global da IDE é gerenciado pelo Zustand e inclui lógica complexa para:
*   **Recursive Layout Updates:** Funções para inserir, remover e mover nós na árvore de layout sem corromper a estrutura.
*   **Theme Engine:** Aplicação dinâmica de variáveis de cor CSS baseadas no tema selecionado.
*   **Project Management:** Sincronização de projetos ativos entre a UI e o sistema de arquivos local através da API do Tauri.

---
*Documentação da Interface v1.0 - Abril 2026*
