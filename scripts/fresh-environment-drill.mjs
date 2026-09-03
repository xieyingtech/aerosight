import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

const requiredEnvironmentKeys = [
  "DATABASE_URL", "AUTH_SECRET", "LOG_LEVEL", "WORKER_NAME", "OBJECT_STORAGE_LOCAL_ROOT",
  "ALGORITHM_ALLOWED_HOSTS", "CALLBACK_LISTEN_ADDRESS", "CALLBACK_PUBLIC_BASE_URL",
  "MEDIA_API_BASE_URL", "MEDIA_ADMIN_USER", "MEDIA_ADMIN_PASSWORD",
  "DJI_FLIGHTHUB_API_BASE_URL", "DJI_FLIGHTHUB_HTTP_TIMEOUT_MS",
  "DJI_FLIGHTHUB_MAX_RETRIES", "DJI_FLIGHTHUB_MAX_PROJECT_PAGES",
  "DJI_FLIGHTHUB_MAX_RESPONSE_BYTES", "DJI_FLIGHTHUB_POLL_INTERVAL_SECONDS",
  "DJI_FLIGHTHUB_RECONCILE_INTERVAL_SECONDS"
];
const example = readFileSync(".env.example", "utf8");
for (const key of requiredEnvironmentKeys) {
  if (!new RegExp(`^${key}=`, "m").test(example)) throw new Error(`.env.example is missing ${key}`);
}
if (/OPENAI_API_KEY=sk-[A-Za-z0-9_-]+/.test(example)) throw new Error(".env.example contains an API key literal");
for (const removedKey of ["AI_PROVIDER", "AI_MODEL", "OPENAI_API_KEY"]) {
  if (new RegExp(`^${removedKey}=`, "m").test(example)) {
    throw new Error(`${removedKey} must be configured through the AI Provider administration database, not the environment`);
  }
}

const commands = [
  {
    name: "configuration contracts",
    command: ["pnpm", "--dir", "apps/web", "exec", "node", "--test",
      "lib/runtime-config.test.ts", "lib/object-storage-core.test.ts",
      "lib/algorithm-provider-policy.test.ts", "lib/agent-provider-registry.test.ts",
      "lib/dependency-health-core.test.ts"]
  },
  { name: "empty, current, legacy and repeated migrations", command: ["pnpm", "test:migrations"] },
  { name: "production web and worker build", command: ["pnpm", "build"] }
];

const results = [];
for (const step of commands) {
  const started = performance.now();
  const result = spawnSync(step.command[0], step.command.slice(1), {
    cwd: process.cwd(),
    env: { ...process.env, GOPROXY: "off", GOSUMDB: "off" },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"]
  });
  process.stdout.write(result.stdout ?? "");
  process.stderr.write(result.stderr ?? "");
  const durationMilliseconds = performance.now() - started;
  results.push({ name: step.name, durationMilliseconds, passed: result.status === 0 });
  if (result.status !== 0) {
    process.stderr.write(`${JSON.stringify({ schemaVersion: 1, passed: false, results }, null, 2)}\n`);
    process.exit(result.status ?? 1);
  }
}

process.stdout.write(`${JSON.stringify({ schemaVersion: 1, generatedAt: new Date().toISOString(),
  assertions: { documentedEnvironmentKeys: requiredEnvironmentKeys.length,
    aiEnvironmentConfigurationAbsent: true, apiKeyLiteralAbsent: true }, results, passed: true }, null, 2)}\n`);
