import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync, openSync, closeSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const id = randomUUID();
const container = `aerosight-mqtt-${id}`;
const output = resolve(root, '.build', `mqtt-lifecycle-${id}`);
const image = 'eclipse-mosquitto:2.1.2-alpine';
mkdirSync(output, { recursive: true });
const password = randomBytes(24).toString('hex');
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 60000 });
  if (result.status !== 0) throw new Error(`docker ${args[0]} failed: ${result.stderr || result.error}`);
  return result.stdout.trim();
}
let started = false;
try {
  docker('image', 'inspect', image);
  writeFileSync(resolve(output, 'mosquitto.conf'), 'listener 1883\nallow_anonymous false\npassword_file /fixture/passwords\nlog_dest stdout\npersistence false\n');
  docker('run', '--rm', '--user', '0', '-v', `${output}:/fixture`, '--entrypoint', 'mosquitto_passwd', image, '-b', '-c', '/fixture/passwords', 'acceptance', password);
  docker('run', '--rm', '--user', '0', '-v', `${output}:/fixture`, '--entrypoint', 'chmod', image, '644', '/fixture/passwords');
  docker('run', '--rm', '-d', '--name', container, '-p', '127.0.0.1::1883', '-v', `${output}:/fixture:ro`, image, 'mosquitto', '-c', '/fixture/mosquitto.conf');
  started = true;
  const port = docker('port', container, '1883/tcp').split(':').at(-1);
  const log = openSync(resolve(output, 'tests.log'), 'w');
  let status;
  try {
    status = await new Promise((resolve, reject) => {
      const child = spawn('go', ['test', '-tags', 'dev', './internal/dji', '-run', '^TestMQTT', '-count=1', '-timeout=90s', '-v'], {
        cwd: root + '/apps/server', stdio: ['ignore', log, log],
        env: { ...process.env, AEROSIGHT_TEST_MQTT_URL: `mqtt://127.0.0.1:${port}`, AEROSIGHT_TEST_MQTT_USER: 'acceptance', AEROSIGHT_TEST_MQTT_PASSWORD: password },
      });
      child.once('error', reject);
      child.once('exit', resolve);
    });
  } finally { closeSync(log); }
  assert.equal(status, 0, `MQTT tests failed; inspect ${output}`);
  writeFileSync(resolve(output, 'result.json'), JSON.stringify({ image, checks: ['MQTT 5 authentication', 'invalid credentials rejected', 'subscription recovery', 'manager shutdown waits for MQTT', 'manager restart reacquires lease'], passed: true }, null, 2));
  console.log(`MQTT lifecycle passed: ${output}`);
} finally {
  if (started) {
    try { writeFileSync(resolve(output, 'broker.log'), docker('logs', container)); }
    finally { docker('stop', container); }
  }
}
