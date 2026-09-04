import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [
    // The router plugin must run before the React plugin.
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    proxy: {
      // Proxy backend calls during development. Keep the trailing slash so
      // SPA routes like "/apis" are NOT swallowed by a bare "/api" prefix.
      "/api/": { target: "http://localhost:8080", changeOrigin: true },
      // Static uploads served by the backend (local storage driver).
      "/uploads/": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  build: {
    // Output into the Go embed directory so the binary serves the SPA.
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    sourcemap: false,
    target: "es2022",
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      output: {
        // Split heavyweight, rarely-changing dependencies into stable chunks so
        // app code changes do not invalidate them in the browser cache.
        //
        // Vite 8 bundles with rolldown, whose deprecated `manualChunks` shim
        // captures each group's dependencies recursively in group order; with
        // `charts` first that pulled React into the charts chunk. Use the
        // native `codeSplitting.groups` (first match wins) and list the base
        // layer first so React lands in `vendor`, not `charts`.
        codeSplitting: {
          groups: [
            {
              name: "vendor",
              test: (id) =>
                id.includes("node_modules/react/") ||
                id.includes("node_modules/react-dom/") ||
                id.includes("node_modules/scheduler/") ||
                id.includes("node_modules/@tanstack/"),
            },
            {
              name: "connect",
              test: (id) =>
                id.includes("node_modules/@connectrpc/") || id.includes("node_modules/@bufbuild/"),
            },
            {
              name: "charts",
              // recharts plus the deps only it uses (listed explicitly with
              // recursive capture off). Shared helpers like clsx stay out;
              // capturing them would force the entry to preload the whole
              // charts chunk, whereas only the dashboard should pay for it.
              // Anything matched earlier by `vendor` stays there.
              test: (id) =>
                id.includes("node_modules/recharts") ||
                id.includes("node_modules/d3-") ||
                id.includes("node_modules/internmap") ||
                id.includes("node_modules/victory-vendor") ||
                id.includes("node_modules/@reduxjs/") ||
                id.includes("node_modules/redux") ||
                id.includes("node_modules/react-redux") ||
                id.includes("node_modules/reselect") ||
                id.includes("node_modules/immer") ||
                id.includes("node_modules/es-toolkit") ||
                id.includes("node_modules/eventemitter3") ||
                id.includes("node_modules/decimal.js-light") ||
                id.includes("node_modules/react-is") ||
                id.includes("node_modules/tiny-invariant") ||
                id.includes("node_modules/use-sync-external-store"),
              includeDependenciesRecursively: false,
            },
          ],
        },
      },
    },
  },
});
