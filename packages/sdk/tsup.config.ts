import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm", "cjs"],
  dts: true,
  sourcemap: true,
  clean: true,
  target: "es2022",
  // The SDK must work in browsers and in Node, so it ships as plain
  // JavaScript with no Node built-ins beyond the fetch API.
  platform: "neutral",
  treeshake: true,
});
