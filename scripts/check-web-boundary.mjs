import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { resolve, relative } from "node:path";

const root = resolve(import.meta.dirname, "..");
const web = resolve(root, "apps/web");
const excluded = new Set(["node_modules", ".next", "out", ".git"]);
async function* files(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (excluded.has(entry.name)) continue;
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) yield* files(path);
    else if (entry.isFile()) yield path;
  }
}
const forbidden = /^(pg|next-auth|@auth\/.*|server-only|ai|@ai-sdk\/.*|next\/(headers|server))$/;
const manifest = JSON.parse(await readFile(resolve(web, "package.json"), "utf8"));
for (const name of Object.keys({ ...manifest.dependencies, ...manifest.devDependencies })) {
  assert(!forbidden.test(name) && name !== "@types/pg", `Web dependency crossed server boundary: ${name}`);
}
let count = 0;
for await (const path of files(web)) {
  if (!/\.[cm]?[jt]sx?$/.test(path)) continue;
  const name = relative(web, path).replaceAll("\\", "/");
  const source = await readFile(path, "utf8");
  assert(!/^app\/(?:.*\/)?route\.[jt]s$/.test(name), `Route Handler returned: ${name}`);
  assert(!/^\s*["']use server["'];?\s*$/m.test(source), `Server Action returned: ${name}`);
  for (const match of source.matchAll(/(?:from\s*|import\s*\(?\s*|require\s*\(\s*)["']([^"']+)["']/g)) {
    assert(!forbidden.test(match[1]), `Server import in ${name}: ${match[1]}`);
    assert(!match[1].includes("contracts/go-migration") && !match[1].includes("scripts/legacy-db"), `Legacy database code imported by Web: ${name}`);
  }
  count++;
}
console.info(`Web boundary passed: ${count} source files; no server dependencies, handlers, actions or legacy database imports.`);
