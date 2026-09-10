import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import * as safety from './device-command-core.ts';

// Execute the frozen service's ordering with a query fixture. Real SQL locking,
// rollback and concurrent requests are covered by Go/PostGIS integration tests.
test('legacy command replay keeps the original result and rechecks grants before reuse', async () => {
  const require = createRequire(new URL('../../../apps/web/package.json', import.meta.url));
  const ts = require('typescript');
  const source = readFileSync(new URL('./device-commands.ts', import.meta.url), 'utf8');
  const id = '11111111-1111-4111-8111-111111111111';
  let command: { id: string; status: string } | undefined;
  let inserts = 0, publications = 0, denied = false;
  const auditInputs: any[] = [];
  const client = { async query(sql: string, values: any[]) {
    if (sql.includes('from devices device')) return { rows: [{ projectId: 3, deviceTypeId: 'dock', status: 'online', availability: 'available', riskLevel: 'low' }] };
    if (sql.includes('from device_connector_bindings')) return { rows: [{ connectorInstanceId: '1', connectorKey: 'simulator.memory', connectorStatus: 'connected', priority: 100 }] };
    if (sql.includes('from device_capability_grants')) return { rows: denied ? [{ actionPattern: 'dock.*', effect: 'deny' }] : [] };
    if (sql.includes('select id::text,status from device_commands')) return { rows: command ? [command] : [] };
    if (sql.includes('from task_runs run')) return { rows: [{ count: 0 }] };
    assert(sql.includes('insert into device_commands'), sql);
    assert.equal(values[0], id); inserts++; command = { id, status: 'dispatchable' }; return { rows: [command] };
  } };
  const exports: any = {};
  const modules: Record<string, any> = {
    'server-only': {}, 'node:crypto': { randomUUID: () => id },
    '@/lib/data': { requireCurrentProjectPermission: async () => ({ user: { id: 1 }, access: { teamId: 2, role: 'owner' } }) },
    '@/lib/audit': { withAuditedProjectWrite: async (input: any, execute: any) => { const result = await execute(client); auditInputs.push(input); return result; } },
    '@/lib/dji-flighthub-device-command-core': {},
    '@/lib/device-command-core': safety, '@/lib/observability': { correlationId: (id: string) => id },
    '@/lib/project-events': { publishProjectEvent: async () => { publications++; } },
  };
  vm.runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText,
    { exports, Error, require: (name: string) => { assert(name in modules, name); return modules[name]; } });
  const input = { projectId: 3, deviceId: 17, capabilityCode: 'dock.debug.control', commandKey: 'debug.open', parameters: {},
    idempotencyKey: 'frozen-command', confirmation: null, reason: 'baseline', requestId: 'baseline-request' };
  // JSON serialization deliberately crosses the VM realm, just as the route does.
  const call = async (value: any) => JSON.parse(JSON.stringify(await exports.submitDeviceCommand(value)));
  assert.deepEqual(await call(input), { id, status: 'dispatchable', reused: false });
  assert.deepEqual(await call({ ...input, parameters: { changed: true } }), { id, status: 'dispatchable', reused: true });
  denied = true;
  await assert.rejects(() => call(input), /DEVICE_CAPABILITY_EXPLICITLY_DENIED/);
  assert.equal(inserts, 1); assert.equal(publications, 1); assert.equal(auditInputs.length, 2);
  assert.equal(auditInputs[0].input.confirmationPresent, false);
  assert.equal(auditInputs[0].input.confirmation, undefined);
});
