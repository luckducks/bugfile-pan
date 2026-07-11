import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Keep the browser on a same-origin API in development. Production uses the
// Go backend's matching /api/upload route.
export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    proxy: {
      "/api/upload": {
        target: "https://u1.bigfile.net",
        changeOrigin: true,
        secure: true,
        rewrite: () => "/v1/upload",
      },
      "/d": {
        target: "https://www.bigfile.net",
        changeOrigin: true,
        secure: true,
      },
    },
  },
});
