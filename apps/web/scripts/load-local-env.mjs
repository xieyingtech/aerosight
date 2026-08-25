import { existsSync } from "node:fs";
import { resolve } from "node:path";

const candidates = [
  resolve(import.meta.dirname, "../.env.local"),
  resolve(import.meta.dirname, "../../../.env.local")
];

export function loadLocalEnvironment() {
  const envFile = candidates.find((candidate) => existsSync(candidate));
  if (envFile) process.loadEnvFile(envFile);
  return envFile;
}
