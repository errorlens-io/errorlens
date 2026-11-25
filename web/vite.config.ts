import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    // During development the Go server runs on :8080.
    proxy: { "/api": "http://localhost:8080" },
  },
});
