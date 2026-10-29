import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["internal/web/assets/src/js/**/*.test.ts"],
  },
});
