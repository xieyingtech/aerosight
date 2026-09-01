import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function run(command, args, options = {}) {
  const useWindowsCommandShell = process.platform === "win32" && command === "pnpm";
  const executable = useWindowsCommandShell ? (process.env.ComSpec ?? "cmd.exe") : command;
  const commandArgs = useWindowsCommandShell ? ["/d", "/s", "/c", command, ...args] : args;
  const result = spawnSync(executable, commandArgs, {
    cwd: root,
    stdio: "inherit",
    ...options
  });
  if (result.error) console.error(`Unable to start ${command}:`, result.error.message);
  if (result.status !== 0) process.exit(result.status ?? 1);
}

run("pnpm", ["--dir", "apps/web", "exec", "node", "--test",
  "lib/algorithm-provider-policy.test.ts",
  "lib/runtime-config.test.ts",
  "lib/object-storage-core.test.ts",
  "lib/outbound-url-policy.test.ts",
  "lib/media-access-core.test.ts",
  "lib/media-ingestion-core.test.ts",
  "lib/project-snapshot.test.ts",
  "lib/project-replay-core.test.ts",
  "lib/replay-policy.test.ts",
  "lib/approval-core.test.ts",
  "lib/task-run-core.test.ts",
  "lib/mission-audit-trace-core.test.ts",
  "lib/retention-cleanup-core.test.ts",
  "lib/report-aggregation-core.test.ts"
]);

run("go", ["test", "./internal/agent", "./internal/dji", "./internal/mission", "./internal/observability"], {
  cwd: resolve(root, "apps/worker"),
  env: { ...process.env, GOPROXY: "off", GOSUMDB: "off" }
});

console.info("Security acceptance passed: SSRF, redaction, callback replay, scope isolation, command idempotency, approval separation, replay control, emergency safety, and evidence retention.");
