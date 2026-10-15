import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import { privacyPage } from "./privacy-page.ts";

export default defineConfig({
  plugins: [react(), tailwindcss(), privacyPage()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  server: {
    // The Go server runs on 8080. Proxying keeps the browser on one origin,
    // which is how production works too. changeOrigin stays false so Go sees
    // the browser's Host, which its cross-origin check compares with Origin.
    proxy: { "/api": { target: "http://localhost:8080", changeOrigin: false } },
  },
  test: {
    environment: "node",
  },
});
