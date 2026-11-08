import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Built assets are embedded into the Go binary (internal/web/dist).
export default defineConfig({
  plugins: [react()],
  base: "./",
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: { outDir: "../internal/web/dist", emptyOutDir: true, assetsDir: "assets", sourcemap: false },
  server: { proxy: { "/api": "http://127.0.0.1:7780", "/app": "http://127.0.0.1:7781" } },
});
