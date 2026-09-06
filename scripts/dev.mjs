import { existsSync } from 'node:fs';
import { spawn, spawnSync } from 'node:child_process';
import { resolve } from 'node:path';

const root=resolve(import.meta.dirname,'..');
const envFile=resolve(root,'.env.local');
if (existsSync(envFile)) process.loadEnvFile(envFile);
process.env.AEROSIGHT_ENV='development';
process.env.HTTP_LISTEN_ADDRESS ??= '127.0.0.1:8080';
process.env.PUBLIC_ORIGIN ??= 'http://localhost:3000';
process.env.GO_API_ORIGIN ??= `http://${process.env.HTTP_LISTEN_ADDRESS}`;
const build=spawnSync(process.execPath,[resolve(root,'scripts/build-server.mjs'),'--dev'],{cwd:root,stdio:'inherit'});
if (build.error) throw build.error;
if (build.status!==0) process.exit(build.status??1);
const executable=resolve(root,`.build/aerosight-dev${process.platform==='win32'?'.exe':''}`);
const children=[
  spawn(executable,['serve'],{cwd:root,env:process.env,stdio:'inherit'}),
  spawn(process.execPath,[resolve(root,'apps/web/node_modules/next/dist/bin/next'),'dev'],{cwd:resolve(root,'apps/web'),env:process.env,stdio:'inherit'})
];
let stopping=false;
function stop(signal='SIGTERM') {if(stopping)return;stopping=true;for(const child of children) if(child.exitCode===null) child.kill(signal);}
for(const signal of ['SIGINT','SIGTERM']) process.on(signal,()=>stop(signal));
for(const child of children) {
  child.on('error',error=>{console.error(error.message);process.exitCode=1;stop();});
  child.on('exit',(code)=>{if(!stopping){process.exitCode=code??1;stop();}});
}
