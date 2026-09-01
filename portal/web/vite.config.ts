import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // kumo は大きめのライブラリのため、警告閾値を引き上げる
    chunkSizeWarningLimit: 800,
  },
})
