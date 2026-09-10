import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import vm from 'node:vm';
import { createHash } from 'node:crypto';

const root = resolve(import.meta.dirname, '..');
const require = createRequire(resolve(root, 'apps/web/package.json'));
const ts = require('typescript');
const records = JSON.parse(readFileSync(resolve(root, 'contracts/go-migration/entrypoints.json'), 'utf8')).records;
const fixturePath = resolve(root, 'contracts/go-migration/route-responses.json');
const payload = { id: '17', projectId: 3, description: null, items: [], runId: '11111111-1111-4111-8111-111111111111' };
class FlightHubConnectionError extends Error { constructor() { super('scope_forbidden'); this.safeCode = 'scope_forbidden'; } }
class ZodError extends Error {}
const fixtures = [];
for (const record of records.filter(record => record.kind === 'route' && !record.target.includes('/api/auth/*'))) {
  assert.equal(createHash('sha256').update(record.originalHandler).digest('hex'), record.sourceHash, record.source);
  for (const mode of ['success', 'denied']) {
    const calls = [];
    const denied = mode === 'denied';
    const dependency = name => (...args) => {
      calls.push(name);
      if (name === 'requireUser') return Promise.resolve({ id: 1 });
      if (name.startsWith('parse') || name === 'assertLiveControlRequest') return {};
      if (name === 'canAccessProjectStream') return Promise.resolve(!denied);
      if (name.startsWith('create') && name.endsWith('Stream')) return ': heartbeat\n\n';
      if (denied) {
        if (name === 'readProjectSituationSnapshot' || name === 'readProjectReplay') return null;
        throw record.target.includes('flighthub') ? new FlightHubConnectionError() : new Error('PROJECT_ACCESS_DENIED');
      }
      if (name === 'readAuthorizedMediaContent') return { body: 'fixture-bytes', contentType: 'image/png', disposition: 'inline' };
      if (name.startsWith('list')) return [];
      return payload;
    };
    const exported = {};
    const context = vm.createContext({ exports: exported, Response, Request, URL, URLSearchParams, Buffer, Error, SyntaxError,
      process: { env: { MEDIA_ADMIN_USER: 'fixture', MEDIA_ADMIN_PASSWORD: 'fixture-password' } },
      require: specifier => {
        if (specifier === 'next/server') return { NextResponse: Response, NextRequest: Request };
        if (specifier === 'node:crypto') return require('node:crypto');
        if (specifier === 'zod') return { ZodError };
        return new Proxy({}, { get: (_, name) => {
          if (name === 'FlightHubConnectionError' || name === 'FlightHubDiscoveryError') return FlightHubConnectionError;
          if (String(name).endsWith('Error')) return class extends Error {};
          if (name === 'db') return { connect: async () => ({ release() {} }) };
          if (name === 'issueMutationInputSchema') return { parse: value => value };
          return dependency(name);
        } });
      },
    });
    const js = ts.transpileModule(record.originalHandler, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
    vm.runInContext(js, context, { filename: record.source, timeout: 1000 });
    const body = record.target === 'POST /api/media-auth'
      ? { action: 'api', user: 'fixture', password: denied ? 'wrong' : 'fixture-password' }
      : { enabled: true, content: '查询项目', capabilityCode: 'dock.debug.control', commandKey: 'debug.open', parameters: {},
        idempotencyKey: 'baseline-idempotency', reason: 'baseline', action: 'resume', expectedVersion: 1 };
    const request = new Request('https://aerosight.test/fixture?cursor=17', {
      method: record.method, headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'baseline-request' },
      ...(record.method === 'GET' ? {} : { body: JSON.stringify(body) }),
    });
    const params = Object.fromEntries(['id', 'providerId', 'definitionId', 'assetId', 'connectorId', 'adapterId', 'identityId', 'deviceId', 'eventId', 'issueId', 'streamId', 'channelId', 'reportId', 'runId', 'sessionId'].map(key => [key, key === 'id' ? '3' : '17']));
    let response;
    try { response = await exported[record.method](request, { params: Promise.resolve(params) }); }
    catch (error) {
      assert(denied && error.message === 'PROJECT_ACCESS_DENIED', `${record.target}: ${error.stack}`);
      fixtures.push({ target: record.target, mode, sourceHash: record.sourceHash, calls, thrown: error.message });
      continue;
    }
    assert(response instanceof Response, record.target);
    if (!denied) assert(response.status < 300 || (response.status === 410 && record.target.includes('/events/:eventId/')), `success fixture failed: ${record.target} ${response.status}`);
    fixtures.push({ target: record.target, mode, sourceHash: record.sourceHash, calls,
      requestBody: record.method === 'GET' ? null : body, serviceResult: payload,
      status: response.status, headers: Object.fromEntries(response.headers), body: await response.text() });
  }
}
const result = { version: 1,
  scope: '冻结旧 Route Handler 的响应包装、状态码、头和服务结果 JSON 序列化；服务依赖为 fixture，不证明数据库业务。Auth.js 两个入口按批准设计替换，由独立会话测试验收。服务 fixture 中的 ID/null/空数组故意保持不同类型。',
  fixtures };
if (process.argv.includes('--write')) writeFileSync(fixturePath, JSON.stringify(result, null, 2) + '\n');
else assert.deepEqual(result, JSON.parse(readFileSync(fixturePath, 'utf8')), 'legacy response fixtures changed');
console.log(`Legacy response fixtures verified: ${fixtures.length} scenarios for ${fixtures.length / 2} methods`);
