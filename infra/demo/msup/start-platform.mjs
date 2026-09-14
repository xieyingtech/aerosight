import {readFileSync,writeFileSync,openSync} from 'node:fs';
import {spawn} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import pg from 'pg';
process.loadEnvFile('.env.local');
const root=process.cwd(), dir=root+'/.build/msup-demo';
const db=new pg.Client({connectionString:process.env.DATABASE_URL});await db.connect();
const name='aerosight_msup_demo';
if(!(await db.query('select 1 from pg_database where datname=$1',[name])).rowCount) await db.query('CREATE DATABASE aerosight_msup_demo');
await db.end();
const url=new URL(process.env.DATABASE_URL);url.pathname='/'+name;
let secrets;try{secrets=JSON.parse(readFileSync(dir+'/platform-secrets.json','utf8'));}catch{secrets={APP_SECRET:randomBytes(32).toString('hex'),CSRF_SECRET:randomBytes(32).toString('base64')};writeFileSync(dir+'/platform-secrets.json',JSON.stringify(secrets),{mode:0o600});}
const env={...process.env,...secrets,DATABASE_URL:url.toString(),AEROSIGHT_ENV:'development',HOST:'127.0.0.1',PORT:'8092',PUBLIC_ORIGIN:'http://127.0.0.1:8092',CALLBACK_PUBLIC_BASE_URL:'https://127.0.0.1:8444',DATA_DIR:dir+'/platform-objects',ALGORITHM_DEVELOPMENT_ENDPOINT:'https://127.0.0.1:8444/infer',ALGORITHM_CA_FILE:dir+'/tls/cert.pem',SSL_CERT_FILE:dir+'/tls/cert.pem',MEDIA_API_BASE_URL:'',MEDIA_ADMIN_USER:'',MEDIA_ADMIN_PASSWORD:'',DJI_FLIGHTHUB_ENABLED:'false'};
writeFileSync(dir+'/platform-env.json',JSON.stringify(env),{mode:0o600});
const log=openSync(dir+'/platform.log','a');const p=spawn(root+'/.build/aerosight',['serve'],{cwd:root,env,detached:true,stdio:['ignore',log,log]});p.unref();writeFileSync(dir+'/platform.pid',String(p.pid));console.log('Demo platform started',p.pid);
