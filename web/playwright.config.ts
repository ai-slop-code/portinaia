import { defineConfig } from "@playwright/test";
import { tmpdir } from "node:os";
import { join } from "node:path";

const runId = `${process.env.USER ?? "test"}-${process.pid}`;
const port = Number(
  process.env.PORTINAIA_E2E_PORT ?? 20_000 + (process.pid % 20_000),
);
const baseURL = `http://127.0.0.1:${port}`;
const controlPath = join(tmpdir(), `portinaia-e2e-control-${runId}`);
process.env.PORTINAIA_E2E_CONTROL_PATH = controlPath;
process.env.PORTINAIA_E2E_PORT = String(port);

export default defineConfig({
  metadata: { e2eControlPath: controlPath },
  testDir: "./tests",
  outputDir: `./test-results/${runId}`,
  use: {
    baseURL,
  },
  webServer: {
    command: "make build && node web/tests/start-server.mjs",
    cwd: "..",
    url: `${baseURL}/api/v1/health/live`,
    timeout: 120_000,
  },
});
