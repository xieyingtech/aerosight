import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { createHmac, randomBytes, randomUUID } from 'node:crypto';
import { closeSync, mkdirSync, openSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const id = randomUUID();
const container = `aerosight-proxy-${id}`;
const output = resolve(root, '.build', `dev-proxy-${id}`);
mkdirSync(output, { recursive: true });
const logPath = resolve(output, 'services.log');
const log = openSync(logPath, 'w');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`docker ${args[0]} failed: ${result.stderr}`);
  return result.stdout.trim();
}
async function freePort() {
  const server = createServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
let app;
let databaseStarted = false;
try {
  docker('run', '--rm', '-d', '--name', container, '-e', 'POSTGRES_PASSWORD=aerosight-test', '-p', '127.0.0.1::5432', 'postgis/postgis:17-3.5');
  databaseStarted = true;
  const dbPort = docker('port', container, '5432/tcp').split(':').at(-1);
  let databaseReady = false;
  for (let i = 0; i < 100; i++) {
    if (spawnSync('docker', ['exec', container, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'], { stdio: 'ignore' }).status === 0) { databaseReady = true; break; }
    await sleep(200);
  }
  assert(databaseReady, 'PostGIS startup timed out');
  const apiPort = await freePort();
  const webPort = await freePort();
  const apiOrigin = `http://127.0.0.1:${apiPort}`;
  const origin = `http://127.0.0.1:${webPort}`;
  const env = { ...process.env, DATABASE_URL: `postgresql://postgres:aerosight-test@127.0.0.1:${dbPort}/postgres`, APP_SECRET: randomBytes(32).toString('hex'), CSRF_SECRET: randomBytes(32).toString('base64'), HOST: '127.0.0.1', PUBLIC_ORIGIN: origin, GO_API_ORIGIN: apiOrigin, PORT: String(apiPort), GIN_MODE: 'release', DATA_DIR: resolve(output, 'objects'), CALLBACK_PUBLIC_BASE_URL: '', MEDIA_API_BASE_URL: '', MEDIA_ADMIN_USER: '', MEDIA_ADMIN_PASSWORD: '', DJI_FLIGHTHUB_ENABLED: 'false' };
  app = spawn(process.execPath, [resolve(root, 'scripts/dev.mjs')], { cwd: root, env, stdio: ['ignore', log, log] });
  app.on('error', error => { console.error(error.message); });
  let ready = false;
  for (let i = 0; i < 120; i++) {
    try {
      const res = await fetch(origin + '/api/auth/csrf', { signal: AbortSignal.timeout(1500) });
      await res.text();
      if (res.ok) { ready = true; break; }
    } catch {}
    assert.equal(app.exitCode, null, `development services exited; see ${logPath}`);
    await sleep(250);
  }
  assert(ready, `Next API proxy startup timed out; see ${logPath}`);
  const cookies = new Map();
  let token;
  const cookieHeader = () => [...cookies].map(([key, value]) => `${key}=${value}`).join('; ');
  async function request(path, status, body, options = {}) {
    const headers = { Cookie: cookieHeader(), ...(body === undefined ? {} : { Origin: origin, 'Content-Type': 'application/json', 'X-CSRF-Token': token ?? '' }), ...options.headers };
    const res = await fetch(origin + path, { method: options.method ?? (body === undefined ? 'GET' : 'POST'), headers, body: body === undefined ? undefined : JSON.stringify(body), redirect: 'manual', signal: AbortSignal.timeout(10000) });
    for (const cookie of res.headers.getSetCookie()) { const pair = cookie.split(';')[0]; const at = pair.indexOf('='); cookies.set(pair.slice(0, at), pair.slice(at + 1)); }
    assert.equal(res.status, status, `${path}: ${await res.clone().text()}`);
    return res;
  }
  await request('/api/auth/session', 401);
  token = (await (await request('/api/auth/csrf', 200)).json()).csrfToken;
  await request('/api/auth/login', 403, { username: 'admin@example.com', password: 'admin' }, { headers: { 'X-CSRF-Token': '' } });
  await request('/api/auth/login', 403, { username: 'admin@example.com', password: 'admin' }, { headers: { Origin: 'http://evil.test' } });
  const login = await request('/api/auth/login', 200, { username: 'admin@example.com', password: 'admin' });
  assert(login.headers.getSetCookie().some(cookie => cookie.startsWith('aerosight_session=') && /httponly/i.test(cookie)), 'session Set-Cookie missing from proxy');
  await request('/api/auth/session', 200);
  const team = await (await request('/api/teams', 201, { name: 'Proxy acceptance' })).json();
  assert(Number.isInteger(team.id) && team.id > 0);
  const project = await (await request('/api/projects', 201, { teamId: team.id, name: 'Proxy project' })).json();
  assert(Number.isInteger(project.id) && project.id > 0);
  const directory = resolve(env.DATA_DIR, 'projects', String(project.id));
  mkdirSync(directory, { recursive: true });
  writeFileSync(resolve(directory, 'fixture.bin'), '0123456789');
  const asset = docker('exec', container, 'psql', '-U', 'postgres', '-Atq', '-c', `insert into assets(project_id,team_id,kind,storage_key,logical_key,mime_type) values(${project.id},${team.id},'video','projects/${project.id}/fixture.bin','fixture.bin','application/octet-stream') returning id`);
  assert(/^\d+$/.test(asset));
  const access = await (await request(`/api/projects/${project.id}/assets/${asset}/access?action=play`, 200)).json();
  const range = await request(access.url, 206, undefined, { headers: { Range: 'bytes=2-5', 'Accept-Encoding': 'gzip' } });
  assert.equal(await range.text(), '2345');
  assert.equal(range.headers.get('content-range'), 'bytes 2-5/10');
  assert.equal(range.headers.get('content-encoding'), null);
  const head = await request(access.url, 200, undefined, { method: 'HEAD' });
  assert.equal(head.headers.get('content-length'), '10');
  const expires = Math.floor(Date.now() / 1000) + 60;
  const signature = createHmac('sha256', env.APP_SECRET).update(`${project.id}.${asset}.1.${expires}`).digest('hex');
  const signedAsset = `/algorithm-assets/${asset}?${new URLSearchParams({ projectId: String(project.id), version: '1', expires: String(expires), signature })}`;
  const algorithmRange = await request(signedAsset, 206, undefined, { headers: { Cookie: '', Range: 'bytes=2-5', 'Accept-Encoding': 'gzip' } });
  assert.equal(await algorithmRange.text(), '2345');
  assert.equal(algorithmRange.headers.get('content-range'), 'bytes 2-5/10');
  assert.equal(algorithmRange.headers.get('content-encoding'), null);
  await request(signedAsset.replace(signature, 'invalid'), 403, undefined, { headers: { Cookie: '' } });
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  const started = Date.now();
  try {
    const stream = await fetch(origin + `/api/projects/${project.id}/events`, { headers: { Cookie: cookieHeader(), 'X-Request-ID': 'dev-proxy-sse-check' }, signal: controller.signal });
    assert.equal(stream.status, 200);
    const reader = stream.body.getReader();
    const first = await reader.read();
    assert(new TextDecoder().decode(first.value).includes(': heartbeat'), 'SSE first frame missing');
    assert(Date.now() - started < 5000, 'SSE first frame buffered by proxy');
    await reader.cancel();
    controller.abort();
  } finally { clearTimeout(timer); }
  let canceled = false;
  for (let i = 0; i < 40; i++) {
    if (readFileSync(logPath, 'utf8').includes('"id":"dev-proxy-sse-check"')) { canceled = true; break; }
    await sleep(100);
  }
  assert(canceled, 'Next did not propagate SSE cancellation to the Go handler');
  await request('/api/auth/logout', 403, {}, { headers: { 'X-CSRF-Token': '' } });
  await request('/api/auth/session', 200);
  await request('/api/auth/logout', 204, {});
  await request('/api/auth/session', 401);
  const goPage = await fetch(apiOrigin + '/login/', { signal: AbortSignal.timeout(2000) });
  assert.equal(goPage.status, 404, 'Go dev unexpectedly serves/proxies frontend');
  console.log('PASS: real Next rewrites preserve CSRF, Cookie/Set-Cookie, login/logout, writes, Range/HEAD, immediate SSE and cancellation; Go dev has no frontend proxy.');
} finally {
  if (app?.pid && app.exitCode === null) {
    if (process.platform === 'win32') spawnSync('taskkill', ['/PID', String(app.pid), '/T', '/F'], { stdio: 'ignore' });
    else app.kill('SIGTERM');
    await Promise.race([new Promise(resolve => app.once('exit', resolve)), sleep(5000)]);
  }
  closeSync(log);
  if (databaseStarted) docker('stop', container);
}
