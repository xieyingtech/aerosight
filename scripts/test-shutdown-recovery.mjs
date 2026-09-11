import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomBytes, randomUUID, createHash } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { createConnection } from 'node:net';
import { startAlgorithmUpstream } from './container-algorithm-upstream.mjs';

const id = randomUUID(), network = `aerosight-shutdown-${id}`;
const database = `${network}-db`, app = `${network}-app`, volume = `${network}-objects`;
const output = resolve(import.meta.dirname, '../.build', `shutdown-recovery-${id}`);
mkdirSync(output, { recursive: true });
const image = process.argv[2] || 'aerosight:unified-acceptance';
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 45000 });
  if (result.status !== 0) throw new Error(`docker ${args[0]}: ${result.stderr || result.error}`);
  return result.stdout.trim();
}
const sql = statement => docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atqc', statement);
async function until(predicate, message, timeout = 15000) {
  const deadline = Date.now() + timeout;
  while (!await predicate()) { assert(Date.now() < deadline, message); await sleep(200); }
}
const env = {
  DATABASE_URL: `postgresql://postgres:shutdown-test@${database}:5432/postgres`,
  AEROSIGHT_ENV: 'production', PUBLIC_ORIGIN: 'https://aerosight.test', HOST: '0.0.0.0', PORT: '8080',
  APP_SECRET: randomBytes(32).toString('hex'), CSRF_SECRET: randomBytes(32).toString('base64'),
  DATA_DIR: '/var/lib/aerosight/objects', GIN_MODE: 'release', DJI_FLIGHTHUB_ENABLED: 'false',
  CALLBACK_PUBLIC_BASE_URL: 'https://aerosight.test', SSL_CERT_FILE: '/tmp/algorithm-ca.pem',
};
let upstream, origin, slowClient;
let networkCreated = false, volumeCreated = false, dbCreated = false, appCreated = false;
async function start(budget) {
  docker('run', '-d', '--name', app, '--network', network, '--read-only', '--tmpfs', '/tmp:rw,mode=1777',
    '--mount', `type=volume,src=${volume},dst=/var/lib/aerosight`, '-p', '127.0.0.1::8080',
    ...Object.entries({ ...env, SHUTDOWN_TIMEOUT: budget }).flatMap(([key,value]) => ['-e', `${key}=${value}`]), image, 'serve');
  appCreated = true;
  upstream.installTrust(app);
  origin = `http://127.0.0.1:${docker('port', app, '8080/tcp').split(':').at(-1)}`;
  await until(async () => { try { return (await fetch(origin + '/readyz', { signal: AbortSignal.timeout(1000) })).status === 200; } catch { return false; } }, 'readiness timed out');
}
try {
  docker('network', 'create', network); networkCreated = true;
  docker('volume', 'create', volume); volumeCreated = true;
  docker('run', '--rm', '--user', '0', '--mount', `type=volume,src=${volume},dst=/var/lib/aerosight`,
    '--entrypoint', 'chown', image, '10001:10001', '/var/lib/aerosight');
  upstream = await startAlgorithmUpstream({ docker, network, output });
  docker('run', '-d', '--name', database, '--network', network, '-e', 'POSTGRES_PASSWORD=shutdown-test', 'postgis/postgis:17-3.5'); dbCreated = true;
  await until(() => spawnSync('docker', ['exec', database, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'], { stdio: 'ignore' }).status === 0, 'database startup timed out');
  await start('1ns');
  const cookies = new Map(); let csrf;
  async function request(path, expected, body) {
    const response = await fetch(origin + path, { method: body === undefined ? 'GET' : 'POST', signal: AbortSignal.timeout(5000),
      headers: { Cookie: [...cookies].map(([k,v]) => `${k}=${v}`).join('; '), Origin: env.PUBLIC_ORIGIN, 'Content-Type': 'application/json', ...(csrf ? { 'X-CSRF-Token': csrf } : {}) },
      body: body === undefined ? undefined : JSON.stringify(body) });
    for (const cookie of response.headers.getSetCookie()) { const [key,...value] = cookie.split(';')[0].split('='); cookies.set(key,value.join('=')); }
    assert.equal(response.status, expected, await response.clone().text()); return response.json();
  }
  csrf = (await request('/api/auth/csrf', 200)).csrfToken;
  await request('/api/auth/login', 200, { username: 'admin@example.com', password: 'admin' });
  const team = await request('/api/teams', 201, { name: 'Shutdown recovery' });
  const project = await request('/api/projects', 201, { teamId: team.id, name: 'Shutdown recovery' });
  assert(Number.isSafeInteger(team.id) && Number.isSafeInteger(project.id));
  const assetKey = `projects/${project.id}/input.png`;
  const bytes = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII=', 'base64');
  docker('exec', app, 'sh', '-c', 'mkdir -p "$1" && printf "%s" "$2" | base64 -d > "$1/input.png"', 'sh', `${env.DATA_DIR}/projects/${project.id}`, bytes.toString('base64'));
  const providerId = Number(sql(`INSERT INTO algorithm_providers(project_id,team_id,name,provider_type,base_url,status,timeout_seconds)
    VALUES(${project.id},${team.id},'Shutdown upstream','http-json','https://algorithm.test:8443/held-run','active',60) RETURNING id`));
  const assetId = Number(sql(`INSERT INTO assets(project_id,team_id,kind,storage_key,logical_key,status,mime_type,checksum_sha256)
    VALUES(${project.id},${team.id},'image','${assetKey}','${assetKey}','available','image/png','${createHash('sha256').update(bytes).digest('hex')}') RETURNING id`));
  const definition = await request(`/api/projects/${project.id}/algorithm-definitions`, 201, {
    definition: { providerId, name: 'Shutdown recovery', capabilityCode: 'detection' },
    configuration: { executionMode: 'synchronous', modelOrProcess: 'test', inputSchema: {}, parametersSchema: {}, outputSchema: {}, protocolConfig: {}, outputMapping: { detectionsPath: 'results' } },
  });
  const { runId: run } = await request(`/api/projects/${project.id}/algorithm-runs`, 202, { configurationSnapshotId: definition.configurationSnapshotId, assetId, parameters: {} });
  assert.match(run, /^[0-9a-f-]{36}$/);
  await until(() => upstream.readRunHold().received === 1, 'algorithm did not reach held upstream');
  const state = () => JSON.parse(sql(`SELECT json_build_object('status',r.status,'finishedAt',r.finished_at,'objectKey',r.raw_result_object_key,
    'attempts',(SELECT count(*) FROM algorithm_run_attempts WHERE algorithm_run_id=r.id),
    'outboxStatus',e.status,'claims',e.attempts,'leaseExpired',e.locked_until < now(),
    'consumptions',(SELECT count(*) FROM outbox_consumptions WHERE event_id=e.event_id))
    FROM algorithm_runs r JOIN outbox_events e ON e.event_type='algorithm.run.requested' AND e.payload_json->>'runId'=r.id::text WHERE r.id='${run}'`));
  const pending = state();
  assert.equal(pending.status, 'queued'); assert.equal(pending.outboxStatus, 'processing'); assert.equal(pending.claims, 1);
  assert.equal(pending.leaseExpired, false);
  // Keep a handler reading an incomplete body so budget exhaustion is deterministic,
  // even when the canceled algorithm transaction drains immediately.
  slowClient = createConnection({ host: '127.0.0.1', port: Number(new URL(origin).port) });
  slowClient.on('error', () => {});
  await new Promise((resolve, reject) => { slowClient.once('connect', resolve); slowClient.once('error', reject); });
  slowClient.write(`POST /api/projects HTTP/1.1\r\nHost: aerosight.test\r\nOrigin: ${env.PUBLIC_ORIGIN}\r\nCookie: ${[...cookies].map(([k,v]) => `${k}=${v}`).join('; ')}\r\nX-CSRF-Token: ${csrf}\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"name":`);
  await sleep(300);
  assert(!slowClient.destroyed, 'incomplete request ended before shutdown');
  const started = Date.now();
  docker('kill', '--signal=TERM', app);
  assert.equal(docker('wait', app), '1', 'exhausted shutdown budget must report failure');
  const stopMs = Date.now() - started;
  assert(stopMs < 5000, 'exhausted budget must not wait for upstream timeout');
  const logs = spawnSync('docker', ['logs', app], { encoding: 'utf8' });
  writeFileSync(resolve(output, 'exhausted.log'), logs.stdout + logs.stderr);
  assert.match(logs.stdout + logs.stderr, /context deadline exceeded/);
  await until(() => upstream.readRunHold().closed === 1, 'upstream connection was not closed');
  assert.deepEqual(state(), pending, 'canceled transaction must not commit partial success');
  assert.equal(sql("SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend'"), '0');
  docker('rm', app); appCreated = false;
  await start('30s');
  await request('/api/auth/session', 200);
  assert.equal(state().leaseExpired, false, 'recovery must exercise a still-live original lease');
  assert.equal(upstream.readRunHold().received, 1, 'work reclaimed before lease expiry');
  // Do not mutate locked_until: exercise the production 30-second lease itself.
  await until(() => state().status === 'succeeded', 'run did not resume after actual lease expiry', 40000);
  const completed = state();
  assert.equal(completed.outboxStatus, 'completed'); assert.equal(completed.claims, 2);
  assert.equal(completed.attempts, 1); assert.equal(completed.consumptions, 1); assert(completed.finishedAt);
  assert.equal(completed.objectKey, `projects/${project.id}/algorithm-runs/${run}/raw-result.json`);
  assert.deepEqual(JSON.parse(docker('exec', app, 'cat', `${env.DATA_DIR}/${completed.objectKey}`)), { results: [] });
  await sleep(1500);
  assert.deepEqual(state(), completed);
  assert.deepEqual(upstream.readRunHold(), { received: 2, closed: 1, applied: 1, runIds: [run, run] });
  docker('kill', '--signal=TERM', app); assert.equal(docker('wait', app), '0');
  writeFileSync(resolve(output, 'result.json'), JSON.stringify({ image: JSON.parse(docker('image','inspect',image))[0].Id, stopMs, run, pending, completed,
    realLeaseExpiry: true, upstream: upstream.readRunHold(), passed: true,
    scope: 'The first upstream request is held without applying effects; an incomplete HTTP body keeps shutdown draining. Shutdown exhausts its 1ns budget, rolls back local work and releases connections. A fresh process waits for the unmodified 30s lease and completes the same run. External providers still need idempotency when an effect precedes a lost response.' }, null, 2));
  console.log(`Shutdown recovery passed: ${output}`);
} finally {
  slowClient?.destroy();
  const cleanup = [];
  if (appCreated) cleanup.push(() => docker('rm', '-f', app));
  if (dbCreated) cleanup.push(() => docker('rm', '-f', database));
  if (upstream) cleanup.push(() => upstream.close());
  if (networkCreated) cleanup.push(() => docker('network', 'rm', network));
  if (volumeCreated) cleanup.push(() => docker('volume', 'rm', volume));
  for (const action of cleanup) { try { action(); } catch (error) { console.error(error.message); process.exitCode = 1; } }
}
