import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');

const requiredEnvironmentKeys = [
  "DATABASE_URL", "APP_SECRET", "LOG_LEVEL", "WORKER_NAME", "DATA_DIR",
  "ALGORITHM_ALLOWED_HOSTS", "CALLBACK_LISTEN_ADDRESS", "CALLBACK_PUBLIC_BASE_URL",
  "CSRF_SECRET", "AEROSIGHT_ENV", "HOST", "PORT", "PUBLIC_ORIGIN", "GO_API_ORIGIN"
];
const example = readFileSync(resolve(root, '.env.example'), "utf8");
for (const key of requiredEnvironmentKeys) {
  if (!new RegExp(`^${key}=`, "m").test(example)) throw new Error(`.env.example is missing ${key}`);
}
if (/OPENAI_API_KEY=sk-[A-Za-z0-9_-]+/.test(example)) throw new Error(".env.example contains an API key literal");
if (/^(AI_PROVIDER|AI_MODEL|OPENAI_API_KEY)=/m.test(example)) throw new Error('AI provider configuration belongs to the managed database configuration');

const commands = [
  {
    name: "configuration contracts",
    command: ["pnpm", "--dir", "apps/web", "exec", "node", "--test",
      "lib/runtime-config.test.ts", "lib/object-storage-core.test.ts",
      "lib/algorithm-provider-policy.test.ts", "lib/stored-ai-provider-policy.test.ts",
      "lib/dependency-health-core.test.ts"]
  },
  { name: "empty, current, legacy and repeated migrations", command: ["pnpm", "test:migrations"] },
  { name: "static frontend and unified Go production build", command: ["pnpm", "build"] }
];

const results = [];
for (const step of commands) {
  const started = performance.now();
  const windows = process.platform === 'win32';
  const executable = windows ? (process.env.ComSpec ?? 'cmd.exe') : step.command[0];
  // All command tokens are fixed above, never supplied by users or environment values.
  const args = windows ? ['/d', '/s', '/c', ...step.command] : step.command.slice(1);
  const result = spawnSync(executable, args, {
    cwd: root,
    env: { ...process.env, GOPROXY: "off", GOSUMDB: "off" },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"]
  });
  if (result.error) process.stderr.write(`${result.error.message}\n`);
  process.stdout.write(result.stdout ?? "");
  process.stderr.write(result.stderr ?? "");
  const durationMilliseconds = performance.now() - started;
  results.push({ name: step.name, durationMilliseconds, passed: result.status === 0 });
  if (result.status !== 0) {
    process.stderr.write(`${JSON.stringify({ schemaVersion: 1, passed: false, results }, null, 2)}\n`);
    process.exit(result.status ?? 1);
  }
}

process.stdout.write(`${JSON.stringify({ schemaVersion: 2, generatedAt: new Date().toISOString(),
  scope: 'configuration, database migration and production build; full business end-to-end is a separate gate',
  assertions: { documentedEnvironmentKeys: requiredEnvironmentKeys.length, noLegacyAIEnvironment: true,
    apiKeyLiteralAbsent: true }, results, passed: true }, null, 2)}\n`);
