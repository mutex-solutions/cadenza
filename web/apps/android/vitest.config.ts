import { defineConfig } from "vitest/config";

// Tests cover the plain TypeScript modules in src/lib, which never import
// react-native, so they run in Node without a phone or emulator.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
