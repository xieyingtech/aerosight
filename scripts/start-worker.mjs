import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { resolve } from "node:path";

const workspaceRoot = resolve(import.meta.dirname, "..");
const envFile = resolve(workspaceRoot, ".env.local");
const executable = resolve(workspaceRoot, ".build/aerosight-worker");

if (existsSync(envFile)) process.loadEnvFile(envFile);
if (!existsSync(executable)) {
  console.error("Worker executable is missing; run `pnpm build:worker` before `pnpm start`.");
  process.exit(1);
}

const worker = spawn(executable, [], {
  cwd: workspaceRoot,
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
