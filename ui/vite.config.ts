import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { viteStaticCopy } from 'vite-plugin-static-copy'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    viteStaticCopy({
      targets: [{
        src: 'node_modules/material-icon-theme/icons/*.svg',
        dest: 'file-icons',
      }],
    }),
  ],
  server: {
    watch: {
      ignored: ['**/*.symbols'],
    },
    proxy: {
      '/api': { target: 'http://127.0.0.1:7373', changeOrigin: true },
      '/events': { target: 'http://127.0.0.1:7373', changeOrigin: true, ws: true },
      '/terminal': { target: 'http://127.0.0.1:7373', changeOrigin: true, ws: true },
    },
  },
})
