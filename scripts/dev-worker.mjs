import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { resolve } from "node:path";

const workspaceRoot = resolve(import.meta.dirname, "..");
const envFile = resolve(workspaceRoot, ".env.local");
if (existsSync(envFile)) process.loadEnvFile(envFile);

const worker = spawn("go", ["run", "./cmd/worker"], {
  cwd: resolve(workspaceRoot, "apps/worker"),
  env: process.env,
  stdio: "inherit"
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => worker.kill(signal));
}

worker.on("error", (error) => {
  console.error(`Unable to start worker: ${error.message}`);
  process.exitCode = 1;
});
worker.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code ?? 1;
});
