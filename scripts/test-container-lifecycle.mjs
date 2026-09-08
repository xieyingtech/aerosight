import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { callbackRecoveryFixture } from './container-callback-recovery.mjs';
import { startAlgorithmUpstream } from './container-algorithm-upstream.mjs';
import { verifyAIFlow } from './container-ai-flow.mjs';
import { startDeviceFixture } from './container-device-flow.mjs';

const root = resolve(import.meta.dirname, '..');
const id = randomUUID();
const network = `aerosight-lifecycle-${id}`;
const upstreamNetwork = `${network}-upstream`;
const database = `${network}-db`, app = `${network}-app`;
const output = resolve(root, '.build', `container-lifecycle-${id}`);
mkdirSync(output, { recursive: true });
const binary = resolve(output, 'aerosight');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 45000 });
  if (result.status !== 0) throw new Error(`docker ${args[0]}: ${result.stderr || result.error}`);
  return result.stdout.trim();
}
const args = process.argv.slice(2);
assert(args.length === 0 || (args.length === 2 && args[0] === '--image' && args[1]), 'usage: test-container-lifecycle.mjs [--image release-image]');
const releaseImage = args[1];
const metadata = JSON.parse(docker('image', 'inspect', releaseImage || 'nginx:alpine'))[0];
const image = metadata.Id;
const arch = metadata.Architecture;
if (releaseImage) {
  assert.equal(metadata.Config.User, '10001:10001', 'release image must define the unprivileged user');
  assert.deepEqual(metadata.Config.Entrypoint, ['/usr/local/bin/aerosight'], 'release image must start its own Go binary');
  assert.deepEqual(metadata.Config.Cmd, ['serve']);
  assert.deepEqual(Object.keys(metadata.Config.ExposedPorts || {}), ['8080/tcp']);
} else {
  for (const script of ['prepare-server.mjs', 'prepare-web.mjs']) {
    const result = spawnSync(process.execPath, [resolve(root, 'scripts', script)], { cwd: root, stdio: 'inherit' });
    assert.equal(result.status, 0, `${script} failed; run pnpm build:web first`);
  }
  await new Promise((resolve, reject) => {
    const child = spawn('go', ['build', '-trimpath', '-o', binary, './cmd/aerosight'], {
      cwd: root + '/apps/server', stdio: 'inherit', env: { ...process.env, GOOS: 'linux', GOARCH: arch, CGO_ENABLED: '0' },
    });
    child.once('error', reject);
    child.once('exit', code => code === 0 ? resolve() : reject(new Error(`Linux build failed: ${code}`)));
  });
}
let networkCreated = false, dbCreated = false, appCreated = false;
let upstreamNetworkCreated = false;
let upstream;
let deviceFixture;
const streamAbort = new AbortController();
try {
  docker('network', 'create', network); networkCreated = true;
  // TEST-NET-3 stays inside an isolated bridge; no production private-host bypass.
  docker('network', 'create', '--internal', '--subnet', '203.0.113.0/24', upstreamNetwork); upstreamNetworkCreated = true;
  upstream = await startAlgorithmUpstream({ docker, network: upstreamNetwork, output });
  deviceFixture = startDeviceFixture({ docker, network, output, root, arch, image });
  docker('run', '-d', '--name', database, '--network', network, '-e', 'POSTGRES_PASSWORD=lifecycle-test', 'postgis/postgis:17-3.5'); dbCreated = true;
  const env = {
    DATABASE_URL: `postgresql://postgres:lifecycle-test@${database}:5432/postgres`,
    AEROSIGHT_ENV: 'production', PUBLIC_ORIGIN: 'https://aerosight.test', HTTP_LISTEN_ADDRESS: '0.0.0.0:8080',
    AUTH_SECRET: randomBytes(32).toString('hex'), CSRF_AUTH_KEY: randomBytes(32).toString('base64'),
    OBJECT_STORAGE_LOCAL_ROOT: '/tmp/objects', GIN_MODE: 'release', DJI_FLIGHTHUB_ENABLED: 'false',
    CALLBACK_PUBLIC_BASE_URL: 'https://aerosight.test', SSL_CERT_FILE: '/tmp/algorithm-ca.pem',
  };
  for (let i = 0; i < 100; i++) {
    if (spawnSync('docker', ['exec', database, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'], { stdio: 'ignore' }).status === 0) break;
    if (i === 99) throw new Error('database startup timed out');
    await sleep(100);
  }
  const binaryOptions = releaseImage ? [] : ['--user', '10001:10001', '--mount', `type=bind,src=${binary},dst=/app/aerosight,readonly`, '-w', '/app', '--entrypoint', '/app/aerosight'];
  docker('run', '-d', '--name', app, '--network', network, '--read-only', '--tmpfs', '/tmp:rw,mode=1777',
    '-p', '127.0.0.1::8080', ...binaryOptions,
    ...Object.entries(env).flatMap(([key, value]) => ['-e', `${key}=${value}`]), image, 'serve'); appCreated = true;
  docker('network', 'connect', upstreamNetwork, app);
  let origin = `http://127.0.0.1:${docker('port', app, '8080/tcp').split(':').at(-1)}`;
  upstream.installTrust(app);
  async function ready() {
    for (let i = 0; i < 100; i++) {
      try { if ((await fetch(origin + '/readyz', { signal: AbortSignal.timeout(1000) })).status === 200) return; } catch {}
      if (docker('inspect', app, '--format', '{{.State.Running}}') !== 'true') throw new Error('application exited before readiness');
      await sleep(100);
    }
    throw new Error('readiness timed out');
  }
  await ready();
  const runtime = docker('exec', app, 'sh', '-c', 'test -z "$(command -v node)" && test -z "$(command -v pnpm)" && cat /proc/1/comm');
  assert.equal(runtime, 'aerosight');
  assert.equal(docker('exec', app, 'id', '-u'), '10001');
  if (releaseImage) {
    const mounts = JSON.parse(docker('inspect', app))[0].Mounts;
    assert(!mounts.some(mount => mount.Type === 'bind'), 'release test must not mount a local binary or source tree');
    docker('exec', app, 'sh', '-c', 'test -s /etc/ssl/certs/ca-certificates.crt && test -s /usr/share/zoneinfo/Asia/Shanghai');
    assert.equal(docker('exec', '-e', 'TZ=Asia/Shanghai', app, 'date', '+%z'), '+0800', 'release image must contain working timezone data');
  }
  const descriptors = docker('exec', app, 'ls', '-l', '/proc/1/fd');
  const socketIDs = new Set([...descriptors.matchAll(/socket:\[(\d+)\]/g)].map(match => match[1]));
  const tcp = docker('exec', app, 'sh', '-c', 'cat /proc/1/net/tcp /proc/1/net/tcp6');
  const listeners = tcp.split('\n').map(line => line.trim().split(/\s+/)).filter(fields => fields[3] === '0A' && socketIDs.has(fields[9]));
  assert.equal(listeners.length, 1, 'Go must own exactly one TCP listener');
  assert(listeners[0][1].endsWith(':1F90'), 'Go must listen on the unified port 8080');
  const page = await fetch(origin + '/login/');
  assert.equal(page.status, 200);
  assert(page.headers.get('content-security-policy'));
  assert((await page.text()).includes('<html'));
  const cookies = new Map(); let csrf;
  async function request(path, expected, body, { timeoutMs = 5000 } = {}) {
    const response = await fetch(origin + path, {
      method: body === undefined ? 'GET' : 'POST', signal: AbortSignal.timeout(timeoutMs),
      headers: { Cookie: [...cookies].map(([k,v]) => `${k}=${v}`).join('; '), Origin: env.PUBLIC_ORIGIN, 'Content-Type': 'application/json', ...(csrf ? { 'X-CSRF-Token': csrf } : {}) },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    for (const cookie of response.headers.getSetCookie()) { const [key, ...value] = cookie.split(';')[0].split('='); cookies.set(key, value.join('=')); }
    assert.equal(response.status, expected, `${path}: ${await response.clone().text()}`);
    return response;
  }
  csrf = (await (await request('/api/auth/csrf', 200)).json()).csrfToken;
  await request('/api/auth/login', 200, { username: 'admin@example.com', password: 'admin' });
  const team = await (await request('/api/teams', 201, { name: 'Lifecycle team' })).json();
  const project = await (await request('/api/projects', 201, { teamId: team.id, name: 'Lifecycle project' })).json();
  const callbacks = await callbackRecoveryFixture({ docker, database, app, project, team, request, upstream });
  await callbacks.beforeStop(origin);
  const ai = await verifyAIFlow({ request, upstream, project });
  const devices = await deviceFixture.verify({ request, database, project });
  const streamCount = 24;
  const streamStart = Date.now();
  const setupTimeout = setTimeout(() => streamAbort.abort(new Error('SSE load setup timeout')), 10000);
  let readers;
  try {
    readers = await Promise.all(Array.from({ length: streamCount }, async () => {
      const stream = await fetch(origin + `/api/projects/${project.id}/events`, { headers: { Cookie: [...cookies].map(([k,v]) => `${k}=${v}`).join('; ') }, signal: streamAbort.signal });
      assert.equal(stream.status, 200);
      const reader = stream.body.getReader();
      assert(new TextDecoder().decode((await reader.read()).value).includes(': heartbeat'));
      return reader;
    }));
  } finally { clearTimeout(setupTimeout); }
  const samples = [];
  for (let batch = 0; batch < 10; batch++) {
    const pending = Promise.all(Array.from({ length: 8 }, async () => {
      const response = await request(`/api/projects/${project.id}/snapshot`, 200);
      assert.equal((await response.json()).project.id, project.id);
    }));
    samples.push(Number(docker('exec', database, 'psql', '-U', 'postgres', '-Atqc', "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend'")));
    await pending;
  }
  assert(samples.every(count => count <= 30), 'HTTP and worker pools exceeded their combined default budget');
  // Keep all streams alive beyond the ordinary 30-second API deadline.
  await sleep(Math.max(0, 32000 - (Date.now() - streamStart)));
  assert(Number.isSafeInteger(project.id) && Number.isSafeInteger(team.id));
  const marker = `load-${randomUUID()}`;
  docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-c', `INSERT INTO project_events(project_id,team_id,event_id,event_type) VALUES(${project.id},${team.id},'${marker}','acceptance.load')`);
  await Promise.all(readers.map(async reader => {
    const receive = async () => {
      const decoder = new TextDecoder(); let text = '';
      while (!text.includes(marker)) {
        const result = await reader.read();
        assert(!result.done, 'SSE ended before the post-deadline event');
        text += decoder.decode(result.value, { stream: true });
      }
    };
    await Promise.race([receive(), sleep(5000).then(() => { throw new Error('SSE did not receive the post-deadline event'); })]);
  }));
  const load = { streamCount, parallelSnapshotRequests: 8, snapshotRequests: 80, heldMs: Date.now() - streamStart, sampledConnections: samples, connectionBudget: 30, postDeadlineEventReceivedByAll: true };
  await ai.beforeStop();
  const started = Date.now();
  docker('kill', '--signal=TERM', app);
  assert.equal(docker('wait', app), '0', 'SIGTERM must exit successfully');
  const stopMs = Date.now() - started;
  assert(stopMs < 30000, 'shutdown exceeded default budget');
  await ai.afterStop();
  const streamEnded = Promise.all(readers.map(async reader => { while (!(await reader.read()).done) {} return true; })).then(() => true);
  assert(await Promise.race([streamEnded, sleep(2000).then(() => false)]), 'SSE did not close');
  const connections = docker('exec', database, 'psql', '-U', 'postgres', '-Atqc', "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend'");
  assert.equal(connections, '0', 'application leaked database connections');
  docker('start', app);
  upstream.installTrust(app);
  origin = `http://127.0.0.1:${docker('port', app, '8080/tcp').split(':').at(-1)}`;
  await ready();
  await request('/api/auth/session', 200);
  await request(`/api/projects/${project.id}/snapshot`, 200);
  const callbackRecovery = await callbacks.afterRestart(origin);
  const aiFlow = await ai.afterRestart();
  const deviceFlow = await devices.afterRestart();
  writeFileSync(resolve(output, 'device-flow.json'), JSON.stringify(deviceFlow, null, 2));
  writeFileSync(resolve(output, 'ai-flow.json'), JSON.stringify(aiFlow, null, 2));
  docker('kill', '--signal=TERM', app);
  assert.equal(docker('wait', app), '0');
  writeFileSync(resolve(output, 'result.json'), JSON.stringify({ image, mode: releaseImage ? 'release-image' : 'mounted-binary', stopMs, load, callbackRecovery, checks: ['production embedded pages', 'no Node or pnpm', 'Go PID 1 and one TCP listener', 'login and writes', 'concurrent SSE and snapshot load within pool budget', 'SSE survives ordinary API deadline and closes on SIGTERM', 'database connections released', 'restart preserves session and project', 'signed callback completion and replay after restart'], passed: true }, null, 2));
  console.log(`Container lifecycle passed: ${output}`);
} finally {
  streamAbort.abort();
  if (appCreated) { try { writeFileSync(resolve(output, 'application.log'), docker('logs', app)); } finally { docker('rm', '-f', app); } }
  if (dbCreated) docker('rm', '-f', database);
  if (deviceFixture) deviceFixture.close();
  if (upstream) upstream.close();
  if (upstreamNetworkCreated) docker('network', 'rm', upstreamNetwork);
  if (networkCreated) docker('network', 'rm', network);
}
