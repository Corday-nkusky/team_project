import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 构建产物默认写入后端嵌入目录，使 `go build` 能把它打进同一个二进制。
// Docker 构建时用 VITE_OUT_DIR=dist 输出到容器内的独立目录再拷入。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: process.env.VITE_OUT_DIR || '../backend/internal/web/dist',
    emptyOutDir: false, // 保留 .gitkeep，避免仓库里出现「目录被删」的噪声
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080',
    },
  },
})
