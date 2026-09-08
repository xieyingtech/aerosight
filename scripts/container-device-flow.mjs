import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

export function startDeviceFixture({ docker, network, output, root, arch, image }) {
  const broker = `${network}-mqtt`, simulator = `${network}-simulator`;
  const password = randomBytes(24).toString('hex');
  const binary = resolve(output, 'dji-simulator');
  const build = spawnSync('go', ['build', '-trimpath', '-o', binary, './cmd/simulator'], {
    cwd: resolve(root, 'apps/server'), encoding: 'utf8', env: { ...process.env, GOOS: 'linux', GOARCH: arch, CGO_ENABLED: '0' },
  });
  assert.equal(build.status, 0, build.stderr);
  const brokerImage = 'eclipse-mosquitto:2.1.2-alpine';
  writeFileSync(resolve(output, 'mosquitto.conf'), 'listener 1883\nallow_anonymous false\npassword_file /fixture/passwords\nlog_dest stdout\npersistence false\n');
  docker('run', '--rm', '--user', '0', '-v', `${output}:/fixture`, '--entrypoint', 'mosquitto_passwd', brokerImage, '-b', '-c', '/fixture/passwords', 'acceptance', password);
  docker('run', '--rm', '--user', '0', '-v', `${output}:/fixture`, '--entrypoint', 'chmod', brokerImage, '644', '/fixture/passwords');
  docker('run', '-d', '--name', broker, '--network', network, '--network-alias', 'mqtt.test', '-v', `${output}:/fixture:ro`, brokerImage, 'mosquitto', '-c', '/fixture/mosquitto.conf');
  let simulatorStarted = false;
  return {
    async verify({ request, database, project }) {
      const sql = statement => docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atqc', statement);
      const wait = async (read, accepts, message) => {
        const deadline = Date.now() + 30000;
        for (;;) {
          const value = read();
          if (accepts(value)) return value;
          assert(Date.now() < deadline, message);
          await new Promise(resolve => setTimeout(resolve, 200));
        }
      };
      const base = `/api/projects/${project.id}/device-adapters`;
      const adapter = await (await request(base + '/dji-setup', 201, {
        name: 'Lifecycle DJI', mode: 'lan', mqttEndpoint: 'mqtt://mqtt.test:1883',
        apiPublicBaseUrl: 'https://algorithm.test:8443/network', websocketPublicUrl: 'wss://algorithm.test:8443/network',
        mediaIngestBaseUrl: 'rtmp://mqtt.test:1883/live', mediaPlaybackBaseUrl: 'https://algorithm.test:8443/media',
        tlsRequired: false, mqttUsername: 'acceptance', mqttPassword: password,
        appId: 'fixture', appKey: 'fixture-key', appLicense: 'fixture-license',
        mediaPublishUser: 'fixture', mediaPublishPassword: 'fixture-password', ntpServerHost: 'mqtt.test', ntpServerPort: '123',
        gatewaySerials: ['GW-ACCEPTANCE'],
      })).json();
      assert.match(adapter.id, /^\d+$/);
      const check = await (await request(`${base}/${adapter.id}/test`, 200, {})).json();
      assert.equal(check.ok, true, JSON.stringify(check));
      await wait(() => sql(`SELECT status FROM device_adapters WHERE id=${adapter.id}`), value => value === 'connected', 'Go adapter did not connect to MQTT');
      const firstEpoch = Number(sql(`SELECT connection_epoch FROM device_adapters WHERE id=${adapter.id}`));
      docker('run', '-d', '--name', simulator, '--network', network, '--read-only', '--user', '10001:10001',
        '--mount', `type=bind,src=${binary},dst=/tmp/dji-simulator,readonly`, '--entrypoint', '/tmp/dji-simulator',
        '-e', `AEROSIGHT_DJI_SIM_MQTT_PASSWORD=${password}`, image, '-mode', 'dji-mqtt', '-product', 'dock2-m3td',
        '-gateway-sn', 'GW-ACCEPTANCE', '-aircraft-sn', 'AIRCRAFT-ACCEPTANCE', '-mqtt-url', 'mqtt://mqtt.test:1883', '-mqtt-username', 'acceptance');
      simulatorStarted = true;
      const devices = () => JSON.parse(sql(`SELECT coalesce(json_agg(json_build_object('id',device_id,'externalId',external_device_id) ORDER BY external_device_id),'[]') FROM device_external_identities WHERE adapter_id=${adapter.id} AND device_id IS NOT NULL`));
      const observed = await wait(devices, rows => rows.some(row => row.externalId === 'AIRCRAFT-ACCEPTANCE') && rows.some(row => row.externalId === 'GW-ACCEPTANCE'), 'real topology was not projected');
      const aircraft = observed.find(row => row.externalId === 'AIRCRAFT-ACCEPTANCE');
      assert(Number.isSafeInteger(aircraft.id));
      const telemetry = () => JSON.parse(sql(`SELECT coalesce((SELECT json_build_object('capturedAt',captured_at,'payload',payload_json) FROM device_latest_telemetry WHERE device_id=${aircraft.id}),'null')`));
      await wait(telemetry, value => value !== null, 'real telemetry was not projected');
      const snapshot = await (await request(`/api/projects/${project.id}/snapshot`, 200)).json();
      assert(JSON.stringify(snapshot).includes('AIRCRAFT-ACCEPTANCE'), 'project snapshot omitted the simulated aircraft');
      return {
        async afterRestart() {
          const cutoff = Date.now();
          const fresh = await wait(telemetry, value => value && Date.parse(value.capturedAt) > cutoff, 'new Go process did not receive fresh MQTT telemetry');
          assert.deepEqual(devices(), observed, 'restart duplicated or replaced device identities');
          const epoch = Number(sql(`SELECT connection_epoch FROM device_adapters WHERE id=${adapter.id}`));
          assert(epoch > firstEpoch, 'restart did not acquire a newer lease');
          return { adapterId: adapter.id, devices: observed, authenticatedMQTT: true, topologyProjected: true,
            telemetryResumedAfterRestart: true, firstEpoch, restartedEpoch: epoch, latestCapturedAt: fresh.capturedAt,
            scope: 'Real Dock 2 MQTT simulator, API setup and reachability checks. Media and NTP endpoints are connectivity fixtures, not playback or time-service acceptance.' };
        },
      };
    },
    close() {
      if (simulatorStarted) {
        const log = spawnSync('docker', ['logs', simulator], { encoding: 'utf8', timeout: 10000 });
        writeFileSync(resolve(output, 'simulator.log'), (log.stdout || '') + (log.stderr || ''));
        docker('rm', '-f', simulator);
      }
      writeFileSync(resolve(output, 'broker.log'), docker('logs', broker)); docker('rm', '-f', broker);
    },
  };
}
