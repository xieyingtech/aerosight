import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

export async function verifyGoCutover({ output, databaseURL, client, scope, password, secret }) {
  const root = resolve(import.meta.dirname, '../..');
  const name = `aerosight-cutover-${randomUUID()}`;
  const binary = resolve(output, 'aerosight-linux');
  const docker = (...args) => {
    const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 45000 });
    assert.equal(result.status, 0, `docker ${args[0]}: ${result.stderr || result.error}`);
    return result.stdout.trim();
  };
  const image = JSON.parse(docker('image', 'inspect', 'nginx:alpine'))[0];
  await new Promise((resolveDone, reject) => {
    const child = spawn('go', ['build', '-trimpath', '-o', binary, './cmd/aerosight'], { cwd: resolve(root, 'apps/server'), stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0', GOOS: 'linux', GOARCH: image.Architecture } });
    child.once('error', reject);
    child.once('exit', code => code === 0 ? resolveDone() : reject(new Error(`Go build failed: ${code}`)));
  });
  const database = new URL(databaseURL); database.hostname = 'host.docker.internal';
  const publicOrigin = 'https://rollback.test';
  const env = { DATABASE_URL: database.href, AUTH_SECRET: secret, CSRF_AUTH_KEY: randomBytes(32).toString('base64'),
    AEROSIGHT_ENV: 'production', PUBLIC_ORIGIN: publicOrigin, HTTP_LISTEN_ADDRESS: '0.0.0.0:8080',
    OBJECT_STORAGE_LOCAL_ROOT: '/tmp/objects', DJI_FLIGHTHUB_ENABLED: 'false', GIN_MODE: 'release' };
  let started = false;
  const abort = new AbortController();
  try {
    docker('run', '-d', '--name', name, '--add-host', 'host.docker.internal:host-gateway', '--user', '10001:10001', '--read-only', '--tmpfs', '/tmp:rw,mode=1777',
      '--mount', `type=bind,src=${binary},dst=/app/aerosight,readonly`, '--entrypoint', '/app/aerosight', '-p', '127.0.0.1::8080',
      ...Object.entries(env).flatMap(([key, value]) => ['-e', `${key}=${value}`]), image.Id, 'serve'); started = true;
    const origin = `http://127.0.0.1:${docker('port', name, '8080/tcp').split(':').at(-1)}`;
    let ready = false;
    for (let n = 0; n < 100; n++) {
      try { const response = await fetch(`${origin}/readyz`, { signal: AbortSignal.timeout(1000) }); await response.text(); if (response.ok) { ready = true; break; } } catch {}
      assert.equal(docker('inspect', name, '--format', '{{.State.Running}}'), 'true', 'Go exited before ready');
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    assert(ready, 'Go readiness timed out');
    const cookies = new Map(); let csrf;
    const cookie = () => [...cookies].map(([key, value]) => `${key}=${value}`).join('; ');
    const request = async (path, expected, body) => {
      const response = await fetch(origin + path, { method: body === undefined ? 'GET' : 'POST', signal: AbortSignal.timeout(5000),
        headers: { Cookie: cookie(), Origin: publicOrigin, 'Content-Type': 'application/json', ...(csrf ? { 'X-CSRF-Token': csrf } : {}) }, body: body === undefined ? undefined : JSON.stringify(body) });
      for (const item of response.headers.getSetCookie()) { const [key, ...value] = item.split(';')[0].split('='); cookies.set(key, value.join('=')); }
      assert.equal(response.status, expected, `Go ${path} failed`);
      return response;
    };
    csrf = (await (await request('/api/auth/csrf', 200)).json()).csrfToken;
    await request('/api/auth/login', 200, { username: 'rollback@example.test', password });
    await request(`/api/projects/${scope.projectId}/snapshot`, 200);
    const project = await (await request('/api/projects', 201, { teamId: scope.teamId, name: 'Go-created rollback project' })).json();
    const stream = await fetch(`${origin}/api/projects/${scope.projectId}/events`, { headers: { Cookie: cookie() }, signal: abort.signal });
    assert.equal(stream.status, 200);
    const reader = stream.body.getReader();
    assert(new TextDecoder().decode((await reader.read()).value).includes(': heartbeat'));
    const start = Date.now();
    docker('kill', '--signal=TERM', name);
    assert.equal(docker('wait', name), '0', 'Go SIGTERM failed');
    const stopMs = Date.now() - start;
    assert(stopMs < 30000, 'Go shutdown exceeded budget');
    const ended = (async () => { while (!(await reader.read()).done) {} return true; })();
    assert(await Promise.race([ended, new Promise(resolve => setTimeout(() => resolve(false), 2000))]), 'Go SSE did not close');
    const remaining = await client.query("select count(*)::integer as count from pg_stat_activity where datname=current_database() and pid<>pg_backend_pid() and backend_type='client backend'");
    assert.equal(remaining.rows[0].count, 0, 'Go database connections remain before rollback');
    return { login: true, snapshot: true, createdProjectId: project.id, sseClosed: true, stopMs, exitCode: 0, connectionsAfterStop: 0, runtimeFixtureImage: image.Id };
  } finally {
    abort.abort();
    if (started) { try { writeFileSync(resolve(output, 'go-serve.log'), docker('logs', name)); } finally { docker('rm', '-f', name); } }
  }
}
