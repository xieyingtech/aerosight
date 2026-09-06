import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, openSync, closeSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer as tlsServer } from 'node:https';
import { request as httpRequest } from 'node:http';
import { createServer } from 'node:net';
import { resolve } from 'node:path';
import { chromium } from 'playwright';

const root = resolve(import.meta.dirname, '..');
const id = randomUUID(), container = `aerosight-browser-${id}`;
const output = resolve(root, '.build', `production-browser-${id}`);
mkdirSync(output, {recursive:true});
const log = openSync(resolve(output,'server.log'),'w');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function command(name, args) {
  const result = spawnSync(name, args, {encoding:'utf8'});
  assert.equal(result.status, 0, `${name}: ${result.stderr || result.error}`);
  return result.stdout.trim();
}
async function freePort() {
  const server = createServer();
  await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
let app, browser, tls, databaseStarted = false;
const sockets = new Set();
try {
  writeFileSync(resolve(output,'openssl.cnf'),'[req]\ndistinguished_name=dn\n[dn]\n');
  command('openssl', ['req','-config',resolve(output,'openssl.cnf'),'-x509','-newkey','rsa:2048','-nodes','-keyout',resolve(output,'key.pem'),'-out',resolve(output,'cert.pem'),'-days','1','-subj','/CN=localhost','-addext','subjectAltName=IP:127.0.0.1,DNS:localhost']);
  command('docker',['run','--rm','-d','--name',container,'-e','POSTGRES_PASSWORD=aerosight-test','-p','127.0.0.1::5432','postgis/postgis:17-3.5']);
  databaseStarted = true;
  const dbPort = command('docker',['port',container,'5432/tcp']).split(':').at(-1);
  let dbReady = false;
  for(let n=0;n<100;n++) {
    if(spawnSync('docker',['exec',container,'pg_isready','-h','127.0.0.1','-U','postgres'],{stdio:'ignore'}).status===0) { dbReady=true;break; }
    await sleep(200);
  }
  assert(dbReady,'PostGIS startup timed out');
  const apiPort = await freePort();
  tls = tlsServer({key:readFileSync(resolve(output,'key.pem')),cert:readFileSync(resolve(output,'cert.pem'))},(req,res)=>{
    const upstream = httpRequest({hostname:'127.0.0.1',port:apiPort,path:req.url,method:req.method,headers:req.headers}, reply=>{
      res.writeHead(reply.statusCode,reply.headers);reply.pipe(res);
    });
    upstream.on('error',()=>{if(!res.headersSent)res.writeHead(502);res.end();});
    res.on('close',()=>upstream.destroy());req.pipe(upstream);
  });
  tls.on('connection',socket=>{sockets.add(socket);socket.on('close',()=>sockets.delete(socket));});
  await new Promise(resolve=>tls.listen(0,'127.0.0.1',resolve));
  const origin=`https://127.0.0.1:${tls.address().port}`;
  app=spawn(resolve(root,'.build',process.platform==='win32'?'aerosight.exe':'aerosight'),['serve'],{
    cwd:output,stdio:['ignore',log,log],env:{...process.env,AEROSIGHT_ENV:'production',DATABASE_URL:`postgresql://postgres:aerosight-test@127.0.0.1:${dbPort}/postgres`,
      AUTH_SECRET:randomBytes(32).toString('hex'),CSRF_AUTH_KEY:randomBytes(32).toString('base64'),PUBLIC_ORIGIN:origin,HTTP_LISTEN_ADDRESS:`127.0.0.1:${apiPort}`,
      OBJECT_STORAGE_LOCAL_ROOT:resolve(output,'objects'),CALLBACK_PUBLIC_BASE_URL:'',MEDIA_API_BASE_URL:'',MEDIA_ADMIN_USER:'',MEDIA_ADMIN_PASSWORD:'',DJI_FLIGHTHUB_ENABLED:'false',GIN_MODE:'release'}
  });
  let ready=false;
  for(let n=0;n<120;n++) {
    try { const res=await fetch(`http://127.0.0.1:${apiPort}/api/auth/csrf`,{signal:AbortSignal.timeout(1000)});await res.text();if(res.ok){ready=true;break;} } catch {}
    assert.equal(app.exitCode,null,'Go exited before readiness');await sleep(250);
  }
  assert(ready,'Go startup timed out');
  browser=await chromium.launch({channel:process.env.PLAYWRIGHT_CHANNEL ?? (process.platform==='win32'?'msedge':'chromium'),headless:true});
  const context=await browser.newContext({ignoreHTTPSErrors:true});
  await context.addInitScript(()=>{
    window.cspViolations=[];
    document.addEventListener('securitypolicyviolation',event=>window.cspViolations.push({directive:event.effectiveDirective,blocked:event.blockedURI}));
  });
  const page=await context.newPage();
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const response=await page.goto(origin+'/login/');
  assert.equal(response.status(),200);
  assert(response.headers()['content-security-policy'].includes("script-src 'self' 'sha256-"));
  await page.getByLabel('邮箱或手机号').fill('admin@example.com');
  await page.getByLabel('密码',{exact:true}).fill('admin');
  await page.getByRole('button',{name:'登录',exact:true}).click();
  await page.waitForURL(url=>url.pathname==='/projects/' || url.pathname==='/projects');
  await page.waitForFunction(()=>!document.body.innerText.includes('正在检查登录状态'));
  await page.getByRole('link',{name:'新建项目',exact:true}).waitFor({state:'visible'});
  assert.deepEqual(errors,[],'hydration/runtime errors');
  assert.deepEqual(await page.evaluate(()=>window.cspViolations),[],'unexpected CSP violations');
  const cookie=(await context.cookies()).find(cookie=>cookie.name==='aerosight_session');
  assert(cookie?.secure && cookie.httpOnly && cookie.sameSite==='Lax','production session cookie policy');
  await page.screenshot({path:resolve(output,'projects.png'),fullPage:true});
  await page.getByRole('link',{name:'团队',exact:true}).click();
  await page.getByRole('button',{name:'新建团队',exact:true}).click();
  await page.getByLabel('团队名称',{exact:true}).fill('Browser acceptance team');
  await page.getByRole('button',{name:'创建团队',exact:true}).click();
  await page.getByRole('link',{name:'Browser acceptance team',exact:true}).waitFor({state:'visible'});
  await page.getByRole('link',{name:'项目',exact:true}).click();
  await page.getByRole('link',{name:'新建项目',exact:true}).click();
  await page.getByLabel('项目名称',{exact:true}).fill('Browser acceptance project');
  await page.getByRole('button',{name:'创建项目',exact:true}).click();
  await page.waitForURL(url=>url.pathname==='/projects/detail/' && Number(url.searchParams.get('projectId'))>0);
  const detailURL=page.url();
  await page.getByRole('heading',{name:'Browser acceptance project',exact:true}).waitFor({state:'visible'});
  await page.reload();
  await page.getByRole('heading',{name:'Browser acceptance project',exact:true}).waitFor({state:'visible'});
  await page.screenshot({path:resolve(output,'created-project.png'),fullPage:true});
  assert.deepEqual(errors,[],'new resource hydration/runtime errors');
  assert.deepEqual(await page.evaluate(()=>window.cspViolations),[],'new resource CSP violations');
  await page.getByRole('button',{name:/admin@example.com/}).click();
  await page.getByRole('menuitem',{name:'退出登录',exact:true}).click();
  await page.waitForURL(url=>url.pathname==='/login/' || url.pathname==='/login');
  assert.equal((await context.request.get(origin+'/api/auth/session')).status(),401,'logout session still valid');
  await page.goto(detailURL);
  await page.waitForURL(url=>url.pathname==='/login/' || url.pathname==='/login');
  await page.getByLabel('邮箱或手机号').fill('admin@example.com');
  await page.getByLabel('密码',{exact:true}).fill('admin');
  await page.getByRole('button',{name:'登录',exact:true}).click();
  await page.waitForURL(url=>url.pathname==='/projects/' || url.pathname==='/projects');
  await page.getByRole('link',{name:/Browser acceptance project/}).waitFor({state:'visible'});
  command('docker',['exec',container,'psql','-U','postgres','-d','postgres','-c',"update sessions set expiry=now()-interval '1 second'"]);
  await page.reload();
  await page.waitForURL(url=>url.pathname==='/login/' || url.pathname==='/login');
  await page.getByRole('button',{name:'登录',exact:true}).waitFor({state:'visible'});
  assert.deepEqual(errors,[],'session lifecycle runtime errors');
  await page.evaluate(()=>{const script=document.createElement('script');script.textContent='window.unapprovedScriptRan = true';document.body.appendChild(script);});
  await page.waitForFunction(()=>window.cspViolations.some(v=>v.directive==='script-src-elem' && v.blocked==='inline'));
  assert.equal(await page.evaluate(()=>window.unapprovedScriptRan),undefined,'unapproved inline script ran');
  writeFileSync(resolve(output,'result.json'),JSON.stringify({passed:true,checks:['production embedded login hydration','secure session cookie','projects navigation','create team/project through UI','post-build detail direct reload','logout and protected navigation','expired session redirects','unapproved inline script blocked'],errors},null,2));
  console.log(`PASS: production browser resources, session lifecycle, hydration and CSP script rejection; evidence ${output}`);
} finally {
  await browser?.close();
  if(tls){for(const socket of sockets)socket.destroy();tls.closeAllConnections();await new Promise(resolve=>tls.close(resolve));}
  if(app?.pid && app.exitCode===null){
    if(process.platform==='win32')spawnSync('taskkill',['/PID',String(app.pid),'/T','/F'],{stdio:'ignore'});else app.kill('SIGTERM');
    await Promise.race([new Promise(resolve=>app.once('exit',resolve)),sleep(5000)]);
  }
  closeSync(log);
  if(databaseStarted)command('docker',['stop',container]);
}
