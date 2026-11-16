import { spawn, spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { clearInterval, setInterval } from "node:timers";
import process from "node:process";

const directory = await mkdtemp(join(tmpdir(), "portinaia-e2e-"));
const database = join(directory, "portinaia.db");
const password = "e2e-administrator-password";
const env = {
  ...process.env,
  PORTINAIA_DB: database,
  PORTINAIA_BOOTSTRAP_PASSWORD: password,
};

const migrate = spawnSync("./bin/portinaia", ["db", "migrate", "-path", database], {
  env,
  stdio: "inherit",
});
if (migrate.status !== 0) {
  await rm(directory, { recursive: true, force: true });
  process.exit(migrate.status ?? 1);
}

const bootstrap = spawnSync(
  "./bin/portinaia",
  ["admin", "bootstrap", "-username", "admin"],
  {
    env,
    stdio: "inherit",
  },
);
if (bootstrap.status !== 0) {
  await rm(directory, { recursive: true, force: true });
  process.exit(bootstrap.status ?? 1);
}

const serverEnv = { ...env };
delete serverEnv.PORTINAIA_BOOTSTRAP_PASSWORD;
const port = process.env.PORTINAIA_E2E_PORT;
if (!port) throw new Error("PORTINAIA_E2E_PORT is required");
const serverArgs = ["server", "-listen", `127.0.0.1:${port}`, "-db", database];
const controlPath = process.env.PORTINAIA_E2E_CONTROL_PATH;
if (!controlPath) throw new Error("PORTINAIA_E2E_CONTROL_PATH is required");
let server;
let shuttingDown = false;
let restarting = false;
async function launchServer() {
  server = spawn("./bin/portinaia", serverArgs, {
    env: serverEnv,
    stdio: "inherit",
  });
  await writeFile(`${controlPath}.pid`, String(server.pid), { mode: 0o600 });
  server.once("exit", async (code) => {
    if (shuttingDown || restarting) return;
    await rm(directory, { recursive: true, force: true });
    process.exit(code ?? 1);
  });
}
await launchServer();

await writeFile(controlPath, "", { mode: 0o600 });
let controlValue = "";
const controlLoop = setInterval(async () => {
  let next;
  try {
    next = await readFile(controlPath, "utf8");
  } catch {
    return;
  }
  if (next.trim() !== "restart" || next === controlValue) return;
  controlValue = next;
  await writeFile(controlPath, "", { mode: 0o600 });
  controlValue = "";
  const previous = server;
  restarting = true;
  await new Promise((resolve) => {
    previous.once("exit", resolve);
    previous.kill("SIGTERM");
  });
  await launchServer();
  restarting = false;
}, 100);

async function stop() {
  shuttingDown = true;
  clearInterval(controlLoop);
  if (server.exitCode === null) {
    await new Promise((resolve) => {
      server.once("exit", resolve);
      server.kill("SIGTERM");
    });
  }
  await rm(directory, { recursive: true, force: true });
  await rm(controlPath, { force: true });
  await rm(`${controlPath}.pid`, { force: true });
}

process.once("SIGINT", () => void stop());
process.once("SIGTERM", () => void stop());
