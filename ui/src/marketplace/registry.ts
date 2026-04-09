import type { ExtensionManifest } from './installer'

// Mock de extensões disponíveis no marketplace
export const MARKETPLACE_EXTENSIONS: (ExtensionManifest & { id: string; installed?: boolean })[] = [
  {
    id: 'dracula-polvo',
    name: 'dracula-polvo',
    version: '1.0.0',
    description: 'Tema Dracula adaptado para o Polvo IDE',
    category: 'theme',
    provides: [
      {
        theme: {
          label: 'Dracula',
          variables: {
            '--bg': '#282a36',
            '--bg-dark': '#21222c',
            '--primary': '#50fa7b',
            '--accent': '#ff79c6',
            '--text': '#f8f8f2',
            '--muted': '#6272a4',
            '--border': '#44475a',
            '--title-bar': '#21222c',
            '--code-bg': '#1e1f29',
            '--error': '#ff5555',
            '--success': '#50fa7b',
            '--info': '#8be9fd',
            '--user': '#bd93f9',
          },
        },
      },
    ],
  },
  {
    id: 'nord-polvo',
    name: 'nord-polvo',
    version: '0.9.0',
    description: 'Tema Nord para o Polvo IDE',
    category: 'theme',
    provides: [
      {
        theme: {
          label: 'Nord',
          variables: {
            '--bg': '#2e3440',
            '--bg-dark': '#242933',
            '--primary': '#88c0d0',
            '--accent': '#bf616a',
            '--text': '#eceff4',
            '--muted': '#4c566a',
            '--border': '#3b4252',
            '--title-bar': '#2e3440',
            '--code-bg': '#292e39',
            '--error': '#bf616a',
            '--success': '#a3be8c',
            '--info': '#81a1c1',
            '--user': '#b48ead',
          },
        },
      },
    ],
  },
  {
    id: 'gemini-provider',
    name: 'gemini-provider',
    version: '1.0.0',
    description: 'Provider Google Gemini para o Polvo',
    category: 'ai-model',
    dock: false,
  },
  {
    id: 'linear-panel',
    name: 'linear-panel',
    version: '0.3.0',
    description: 'Painel integrado ao Linear issue tracker',
    category: 'tools',
    dock: true,
    panelType: 'linear',
  },
]
