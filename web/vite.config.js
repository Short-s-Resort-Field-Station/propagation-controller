import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
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
      '/api': 'http://localhost:8090',
    },
  },
});
