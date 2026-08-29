import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/admin/",
  clearScreen: false,
  server: {
    port: 5180,
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:9318",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
  },
});