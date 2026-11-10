import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  // Served under /propagate/ (public reverse proxy; the Go server mounts the
  // same tree at /propagate/ and redirects / and /ui/touch there). Every asset
  // URL the build emits is prefixed, so the bundle works at either origin.
  base: '/propagate/',
  build: {
    outDir: 'dist',
    rollupOptions: {
      input: {
        // "/" — the paper control center (Surface B)
        main: resolve(__dirname, 'index.html'),
        // "/ui/touch" — the rack instrument face (Surface A · Round 2)
        touch: resolve(__dirname, 'touch.html'),
      },
    },
  },
  server: {
    proxy: {
      '/propagate/api': 'http://localhost:8090',
      '/api': 'http://localhost:8090',
    },
  },
  test: {
    environment: 'jsdom',
    globals: false,
    include: ['src/**/*.test.{js,jsx}'],
  },
});
