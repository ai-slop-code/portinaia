import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  use: {
    baseURL: "http://127.0.0.1:8080",
  },
  webServer: {
    command: "make build && ./bin/portinaia server",
    cwd: "..",
    url: "http://127.0.0.1:8080/api/v1/health/live",
    timeout: 120_000,
  },
});
