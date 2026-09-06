import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { readdir, readFile, writeFile, mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve, join, relative } from 'node:path';

const version = 'v1.31.1';
const root = resolve(import.meta.dirname, '..');
const cwd = resolve(root, 'apps/server');
const toolDir = resolve(root, '.build/tools');
const executable = resolve(toolDir, process.platform === 'win32' ? 'sqlc.exe' : 'sqlc');
const check = process.argv.includes('--check');
function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', ...options });
  if (result.error || result.status !== 0) throw new Error(result.error?.message ?? result.stderr ?? `${command} failed`);
  return result.stdout;
}
await mkdir(toolDir, { recursive: true });
if (!existsSync(executable) || run(executable, ['version']).trim() !== version) {
  run('go', ['install', `github.com/sqlc-dev/sqlc/cmd/sqlc@${version}`], { env: { ...process.env, GOBIN: toolDir } });
}
if (!check) {
  run(executable, ['generate']);
  console.log(`Generated queries with sqlc ${version}`);
} else {
  const prefix = join(root, '.build', 'sqlc-check-');
  const scratch = await mkdtemp(prefix);
  try {
    const config = (await readFile(resolve(cwd, 'sqlc.yaml'), 'utf8'))
      .replace('"../../db/schema.sql"', JSON.stringify(relative(scratch, resolve(root, 'db/schema.sql')).replaceAll('\\', '/')))
      .replace('"internal/database/queries"', JSON.stringify(relative(scratch, resolve(cwd, 'internal/database/queries')).replaceAll('\\', '/')))
      .replace('"internal/database/sqlcgen"', '"generated"');
    await writeFile(join(scratch, 'sqlc.yaml'), config);
    run(executable, ['generate', '-f', join(scratch, 'sqlc.yaml')]);
    const actual = resolve(cwd, 'internal/database/sqlcgen');
    const expected = join(scratch, 'generated');
    const names = (await readdir(expected)).sort();
    if (JSON.stringify(names) !== JSON.stringify((await readdir(actual)).sort())) throw new Error('sqlc file list drift: run pnpm db:generate');
    for (const name of names) if (!(await readFile(join(expected, name))).equals(await readFile(join(actual, name)))) throw new Error(`sqlc drift in ${name}: run pnpm db:generate`);
    console.log(`sqlc ${version}: generated files match`);
  } finally {
    // Only remove the concrete directory returned by mkdtemp in our dedicated prefix.
    if (!scratch.startsWith(prefix)) throw new Error('Unexpected temporary directory');
    await rm(scratch, { recursive: true, force: true });
  }
}
