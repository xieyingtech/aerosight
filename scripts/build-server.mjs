import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const development = process.argv.includes('--dev');
for (const name of ['prepare-server.mjs', ...development ? [] : ['prepare-web.mjs']]) {
  const result = spawnSync(process.execPath, [resolve(root,'scripts',name)], {cwd:root,stdio:'inherit'});
  if (result.error) throw result.error;
  if (result.status!==0) process.exit(result.status??1);
}
mkdirSync(resolve(root,'.build'),{recursive:true});
const output = resolve(root,`.build/aerosight${development?'-dev':''}${process.platform==='win32'?'.exe':''}`);
const result = spawnSync('go', ['build', ...development?['-tags','dev']:[], '-trimpath','-o',output,'./cmd/aerosight'], {cwd:resolve(root,'apps/server'),stdio:'inherit'});
if (result.error) throw result.error;
if (result.status!==0) process.exit(result.status??1);
console.log(`Built ${output}`);
