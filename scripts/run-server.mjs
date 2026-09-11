import { existsSync } from 'node:fs';
import { spawn, spawnSync } from 'node:child_process';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname,'..');
const mode = process.argv[2] ?? 'serve';
if (!['serve','migrate','dev'].includes(mode)) throw new Error('Expected serve, migrate, or dev');
const envFile = resolve(root,'.env.local');
if (existsSync(envFile)) process.loadEnvFile(envFile);
const development = mode !== 'serve';
if (mode==='dev') {
  process.env.AEROSIGHT_ENV='development';
  process.env.HOST ??= '127.0.0.1';
  process.env.PORT ??= '8080';
  process.env.PUBLIC_ORIGIN ??= 'http://localhost:3000';
}
if (development) {
  const build=spawnSync(process.execPath,[resolve(root,'scripts/build-server.mjs'),'--dev'],{cwd:root,stdio:'inherit'});
  if (build.error) throw build.error;
  if (build.status!==0) process.exit(build.status??1);
}
const executable=resolve(root,`.build/aerosight${development?'-dev':''}${process.platform==='win32'?'.exe':''}`);
if (!existsSync(executable)) throw new Error('Application executable is missing; run pnpm build first');
const child=spawn(executable,[mode==='migrate'?'migrate':'serve'],{cwd:root,env:process.env,stdio:'inherit'});
for (const signal of ['SIGINT','SIGTERM']) process.on(signal,()=>child.kill(signal));
child.on('error',error=>{console.error(`Unable to start application: ${error.message}`);process.exitCode=1;});
child.on('exit',(code,signal)=>{process.exitCode=code??(signal==='SIGINT'?130:1);});
