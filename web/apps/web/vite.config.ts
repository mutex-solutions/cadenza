import { defineConfig } from "vite";

// The Go server embeds dist/ and serves it at "/", with unknown paths
// falling back to index.html (thread URLs are /t/<id>).
export default defineConfig({
  server: {
    proxy: {
      "/v1": { target: "http://localhost:8080", ws: false },
      "/blob": { target: "http://localhost:8080" },
    },
  },
  build: {
    outDir: "dist",
    target: "es2022",
    sourcemap: false,
    modulePreload: { polyfill: false },
    chunkSizeWarningLimit: 4000,
  },
  test: {
    environment: "happy-dom",
    setupFiles: ["./src/test/setup.ts"],
    // Let styles.test.ts read the real stylesheet (other CSS stays stubbed).
    css: { include: [/styles\.css/] },
  },
});
