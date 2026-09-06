import { readdir, readFile, mkdir, writeFile, unlink, lstat } from 'node:fs/promises';
import { resolve, relative, dirname, sep } from 'node:path';
import { pageCSPEntry } from './web-csp.mjs';

const root = resolve(import.meta.dirname, '..');
const source = resolve(root, 'apps/web/out');
const target = resolve(root, 'apps/server/internal/webassets/dist');
async function filesIn(directory, prefix = '') {
  const files = [];
  for (const entry of await readdir(directory, {withFileTypes:true})) {
    const name = prefix + entry.name;
    if (entry.isSymbolicLink()) throw new Error(`Symlink in web assets: ${name}`);
    if (entry.isDirectory()) files.push(...await filesIn(resolve(directory,entry.name), name+'/'));
    else if (entry.isFile()) files.push(name);
    else throw new Error(`Non-regular web asset: ${name}`);
  }
  return files.sort();
}
if ((await lstat(source)).isSymbolicLink()) throw new Error('Static export source is a symlink');
const names = await filesIn(source);
for (const name of ['index.html','login/index.html','projects/index.html','404.html']) {
  if (!names.includes(name) || !(await readFile(resolve(source,name))).length) throw new Error(`Missing static export: ${name}`);
}
if (!names.some(name=>name.startsWith('_next/static/') && name.endsWith('.js'))) throw new Error('Missing Next.js chunks');
function destination(name) {
  const result = resolve(target,name);
  if (relative(target,result).startsWith('..'+sep) || result===target) throw new Error('Invalid web asset path');
  return result;
}
await mkdir(target,{recursive:true});
if ((await lstat(target)).isSymbolicLink()) throw new Error('Web asset destination is a symlink');
const old = await filesIn(target);
const cspPages = {};
for (const name of names) {
  await mkdir(dirname(destination(name)),{recursive:true});
  const bytes = await readFile(resolve(source,name));
  await writeFile(destination(name), bytes);
  if (name.endsWith('.html')) cspPages[name] = pageCSPEntry(bytes);
}
const cspName = 'csp-manifest.json';
await writeFile(destination(cspName), JSON.stringify({version:1,pages:cspPages})+'\n');
for (const name of old) if (!names.includes(name) && name !== cspName) await unlink(destination(name));
console.log(`Prepared ${names.length} static export files for Go embed`);
