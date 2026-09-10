import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, openSync, closeSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const id = randomUUID(), name = `aerosight-media-inspector-${id}`;
const output = resolve(root, '.build', `media-inspector-${id}`);
mkdirSync(output, { recursive: true });
const image = 'bluenviron/mediamtx:1.20.1', path = 'demo/aerosight/inspector';
const password = randomBytes(24).toString('hex');
const logs = [];
let started = false, publisher;
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000 });
  assert.equal(result.status, 0, `docker ${args[0]}: ${result.stderr || result.error}`);
  return result.stdout.trim();
}
function start(command, args, env, filename) {
  const log = openSync(resolve(output, filename), 'w'); logs.push(log);
  return spawn(command, args, { cwd: resolve(root, 'apps/server'), env: { ...process.env, ...env }, stdio: ['ignore', log, log] });
}
try {
  writeFileSync(resolve(output, 'mediamtx.yml'), `api: true
apiAddress: :9997
authMethod: internal
authInternalUsers:
  - user: acceptance
    pass: ${password}
    permissions:
      - action: api
  - user: any
    permissions:
      - action: publish
        path: ${path}
rtspTransports: [tcp]
rtmp: false
hls: false
webrtc: false
srt: false
moq: false
paths:
  ${path}: {}
`);
  docker('run', '--rm', '-d', '--name', name, '-p', '127.0.0.1::8554', '-p', '127.0.0.1::9997', '-v', `${output}:/fixture:ro`, image, '/fixture/mediamtx.yml'); started = true;
  const api = `http://127.0.0.1:${docker('port', name, '9997/tcp').split(':').at(-1)}`;
  const rtspPort = docker('port', name, '8554/tcp').split(':').at(-1);
  let publisherError;
  publisher = start('ffmpeg', ['-hide_banner', '-loglevel', 'warning', '-re', '-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=24', '-an', '-c:v', 'libx264', '-preset', 'ultrafast', '-tune', 'zerolatency', '-pix_fmt', 'yuv420p', '-f', 'rtsp', '-rtsp_transport', 'tcp', `rtsp://127.0.0.1:${rtspPort}/${path}`], {}, 'publisher.log');
  publisher.on('error', error => { publisherError = error; });
  let received = false;
  for (let n = 0; n < 60; n++) {
    assert(!publisherError && publisher.exitCode === null, 'synthetic publisher failed');
    try {
      const response = await fetch(`${api}/v3/paths/list`, { headers: { Authorization: `Basic ${Buffer.from(`acceptance:${password}`).toString('base64')}` }, signal: AbortSignal.timeout(1000) });
      const body = await response.json();
      if (response.ok && body.items?.some(item => item.name === path && item.tracks?.length)) {
        writeFileSync(resolve(output, 'paths.json'), JSON.stringify(body, null, 2)); received = true; break;
      }
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert(received, 'MediaMTX did not receive video');
  const denied = await fetch(`${api}/v3/paths/list`); assert.equal(denied.status, 401); await denied.text();
  const code = await new Promise((resolveDone, reject) => {
    const child = start('go', ['test', '-tags', 'dev', './internal/dji', '-run', '^TestMediaMTXInspectorIntegration$', '-count=1', '-v'], {
      AEROSIGHT_TEST_MEDIAMTX_API_URL: api, AEROSIGHT_TEST_MEDIAMTX_API_USER: 'acceptance', AEROSIGHT_TEST_MEDIAMTX_API_PASSWORD: password, AEROSIGHT_TEST_MEDIAMTX_READY_PATH: path,
    }, 'tests.log');
    child.once('error', reject); child.once('exit', resolveDone);
  });
  assert.equal(code, 0, `Media inspector failed; inspect ${output}`);
  writeFileSync(resolve(output, 'result.json'), JSON.stringify({ passed: true, image, path, authenticatedAPI: true, realH264Input: true }, null, 2));
  console.log(`Media inspector passed: ${output}`);
} finally {
  if (publisher?.pid && publisher.exitCode === null && publisher.signalCode === null) {
    const ended = new Promise(resolve => publisher.once('exit', resolve)); publisher.kill(); await ended;
  }
  for (const log of logs) closeSync(log);
  if (started) { try { writeFileSync(resolve(output, 'media.log'), docker('logs', name)); } finally { docker('stop', name); } }
}
