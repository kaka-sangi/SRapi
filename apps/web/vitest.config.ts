/// <reference types="vitest" />
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "url";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "happy-dom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["tests/unit/**/*.{test,spec}.{ts,tsx}"],
  },
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
      "@srapi/sdk": fileURLToPath(new URL("../../packages/sdk/typescript/src/index.ts", import.meta.url)),
      "@srapi/sdk/client": fileURLToPath(
        new URL("../../packages/sdk/typescript/src/client.gen.ts", import.meta.url),
      ),
      "@srapi/sdk/core": fileURLToPath(
        new URL("../../packages/sdk/typescript/src/core/index.ts", import.meta.url),
      ),
    },
  },
});
