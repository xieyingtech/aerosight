import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const root = resolve(import.meta.dirname, '..');
const directory = resolve(root, 'contracts/go-migration/legacy-web');
const commit = 'bc314b3731ebc27b301ce0e15d0ffd50ab090a40';
const names = ['project-permission-policy', 'credential-encryption', 'audit-boundary', 'device-command-core'];
const sourceHash = path => createHash('sha256').update(readFileSync(path, 'utf8').replace(/\r\n/g, '\n')).digest('hex');
const sources = [];
for (const name of names) for (const suffix of ['.ts', '.test.ts']) {
  const source = `apps/web/lib/${name}${suffix}`;
  const local = resolve(directory, name + suffix);
  if (process.argv.includes('--capture')) {
    const result = spawnSync('git', ['show', `${commit}:${source}`], { cwd: root, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    writeFileSync(local, result.stdout);
  }
  sources.push({ source, sha256: sourceHash(local) });
}
const policy = await import(pathToFileURL(resolve(directory, 'project-permission-policy.ts')));
const cases = [];
for (const role of ['owner', 'admin', 'member']) {
  for (const grants of [[], ...policy.PROJECT_PERMISSIONS.map(permission => [permission]), ['unknown:permission'], ['event:handle', 'event:handle', 'unknown:permission'], [...policy.PROJECT_PERMISSIONS]]) {
    cases.push({ role, grants, permissions: [...policy.effectiveProjectPermissions(role, grants)].sort() });
  }
}
const commandSource = 'apps/web/lib/device-commands.ts';
const commandLocal = resolve(directory, 'device-commands.ts');
if (process.argv.includes('--capture')) {
  const result = spawnSync('git', ['show', `${commit}:${commandSource}`], { cwd: root, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr); writeFileSync(commandLocal, result.stdout);
}
sources.push({ source: commandSource, sha256: sourceHash(commandLocal) });
const fixture = { commit, sources, cases };
const path = resolve(root, 'contracts/go-migration/permission-matrix.json');
if (process.argv.includes('--capture')) writeFileSync(path, JSON.stringify(fixture, null, 2) + '\n');
else assert.deepEqual(fixture, JSON.parse(readFileSync(path, 'utf8')), 'legacy permission baseline changed');
console.log(`Legacy permission matrix verified: ${cases.length} role/grant cases; ${sources.length} original source/test hashes`);
