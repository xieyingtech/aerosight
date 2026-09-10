import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
// Ordered mapping, reviewed against the HTTP assertions and the frozen inventory.
// This checks inventory completeness and test results, not assertion quality.
const groups = [
  [/^(static layout|client-only helper)$/, ['page_redirects', 'static_pages'], '静态导航、链接与页面边界（另有生产浏览器）'],
  [/\/api\/auth\//, ['auth', 'csrf_boundary', 'session_response'], '认证替换、会话与 CSRF'],
  [/\/admin\/ai-providers/, ['ai_providers', 'ai_upstream'], '管理、凭据、默认项、上游与撤权'],
  [/\/algorithm-providers/, ['algorithm_providers'], '配置、凭据、脱敏、探测与权限'],
  [/\/algorithm-definitions/, ['algorithm_definitions'], '定义与不可变快照'],
  [/\/algorithm-runs/, ['algorithm_runs'], '创建、详情、重试、事务与作用域'],
  [/\/assets\/.*\/(access|content)/, ['media_access'], '签名、Range/HEAD、期限与租户'],
  [/\/connectors\/dji-flighthub|\/features/, ['flighthub'], '连接生命周期、错误、历史、加密与权限'],
  [/\/device-adapters\/dji-setup|\/device-adapters\/:adapterId\/test/, ['device_network'], '网络探测、配置、回滚与撤权'],
  [/\/device-adapters\/discoveries/, ['device_binding'], '并发绑定、作用域与回滚'],
  [/\/device-adapters/, ['device_adapters'], '适配器读写、凭据与原子创建'],
  [/\/devices\/:deviceId\/commands/, ['device_commands'], '能力、安全、幂等、并发、审计与回滚'],
  [/\/devices\/:deviceId\/live-streams/, ['live_start'], '直播创建、并发与原子分发'],
  [/\/events\/:eventId/, ['legacy_events'], '旧事件详情及废弃写入口 410'],
  [/\/realtime-channels\/|\/events$/, ['streams'], 'SSE 游标、溢出、撤权与断流'],
  [/\/issues.*\/actions/, ['issue_writes'], '协作、状态版本、审计与并发'],
  [/\/issues/, ['issue_reads'], '案件详情、关联证据与权限'],
  [/\/live-streams\/.*\/playback/, ['live_playback'], '播放授权与令牌'],
  [/\/live-streams\/.*\/stop/, ['live_stop'], '停止、并发与回滚'],
  [/\/replay|\/snapshot|\/device-tree/, ['snapshot', 'project_reads'], 'PostGIS、设备树、回放筛选与租户'],
  [/\/reports\/.*\/(export|publish)/, ['reports'], '报告发布、保留与导出'],
  [/\/task-runs\/.*\/reports/, ['report_drafts'], '报告草稿、聚合与回滚'],
  [/\/task-runs\/.*\/(audit-trace|emergency-stop-drill)/, ['mission_audit'], '审计关联与无设备副作用演练'],
  [/\/task-runs\/.*\/control/, ['mission_control'], '状态机、审批、并发与事务'],
  [/\/task-runs/, ['mission_reads'], '任务定义和工作台读取'],
  [/\/agent-sessions\/.*\/messages/, ['agent_chat', 'agent_read_tools'], '工具循环、历史、限制、故障与作用域'],
  [/\/agent-sessions/, ['agent_sessions'], '会话与消息读取、权限和输入'],
  [/\/media-auth/, ['media_auth'], '媒体机器鉴权'],
  [/\/projects\/:id\/:kind/, ['assets_list', 'mission_reads'], '原 listProjectItems 的资产与任务列表'],
  [/\/api\/(teams|projects|profile|admin)(\b|\/)/, ['directory', 'auth'], '目录、表单、管理、空数组与租户'],
];
const inventory = JSON.parse(readFileSync(resolve(root, 'contracts/go-migration/entrypoints.json'), 'utf8')).records;
const rows = inventory.flatMap(record => record.kind === 'page'
  ? (record.queries.length ? record.queries.map(query => ({ ...record, target: query.target })) : [{ ...record, target: 'static layout' }])
  : [record]);
const resultsIndex = process.argv.indexOf('--results');
let passed;
if (resultsIndex >= 0) {
  const events = readFileSync(resolve(root, process.argv[resultsIndex + 1]), 'utf8').trim().split(/\r?\n/).map(line => JSON.parse(line.replace(/^\uFEFF/, '')));
  assert(!events.some(event => event.Action === 'fail'), 'Go regression contains failures');
  assert(events.some(event => event.Package === 'aerosight/server/internal/httpapi' && event.Action === 'pass' && !event.Test), 'HTTP package did not complete');
  passed = new Set(events.filter(event => event.Action === 'pass' && event.Package === 'aerosight/server/internal/httpapi').map(event => event.Test));
}
const lines = ['# 迁移入口验收矩阵', '',
  '基于冻结入口逐项映射到实际 HTTP/PostGIS 测试。测试文件名对应 apps/server/internal/httpapi；每个文件的全部具名测试须在真实数据库回归中通过。该矩阵是覆盖索引，业务断言与运行证据仍以测试正文和回归日志为准。', '',
  '页面行为另由 scripts/browser-page-states.mjs、browser-project-workspaces.mjs、browser-project-details.mjs 与 test-production-browser.mjs 验证加载/失败/拒绝、旧链接及构建后新增资源。Auth.js 两个方法按设计替换为显式 auth API；历史事件写方法保留 410。', '',
  '| 原入口 | 目标 | 验证范围 | 测试文件 |', '| --- | --- | --- | --- |'];
const allTests = new Set();
for (const row of rows) {
  const identity = row.source + (row.name ? '#' + row.name : '');
  const group = groups.find(([pattern]) => pattern.test(row.target));
  if (!group) {
    assert(['static layout', 'client-only helper'].includes(row.target), `unmapped entry: ${identity} ${row.target}`);
    lines.push(`| ${identity} | ${row.target} | 静态导航/链接；无服务查询 | page_redirects_test.go、static_pages_test.go；生产浏览器 |`);
    continue;
  }
  const [, files, scope] = group;
  for (const file of files) {
    const source = readFileSync(resolve(root, `apps/server/internal/httpapi/${file}_test.go`), 'utf8');
    const tests = [...source.matchAll(/^func (Test\w+)\(/gm)].map(match => match[1]);
    assert(tests.length > 0, `no tests in ${file}`);
    for (const name of tests) { allTests.add(name); if (passed) assert(passed.has(name), `missing real regression pass: ${name}`); }
  }
  lines.push(`| ${identity} | ${row.target} | ${scope} | ${files.map(file => file + '_test.go').join('、')} |`);
}
assert.equal(inventory.filter(row => row.kind === 'route').length, 50);
assert.equal(inventory.filter(row => row.kind === 'action').length, 4);
lines.push('', `共 ${inventory.length} 个入口记录、${rows.length} 个方法/页面查询映射，关联 ${allTests.size} 个 Go 顶层测试。`, '',
  '复核命令：node scripts/check-migration-coverage.mjs --results .build/final-go-postgis.jsonl。先执行真实 PostGIS 全包回归，再核对结果；不接受默认未配置数据库时的 skip 作为通过。', '');
const path = resolve(root, 'contracts/go-migration/acceptance-matrix.md');
const content = lines.join('\n');
if (process.argv.includes('--write')) writeFileSync(path, content);
else assert.equal(readFileSync(path, 'utf8').replace(/\r\n/g, '\n'), content, 'acceptance matrix drift');
console.log(`Migration coverage: ${rows.length} mapped entries, ${allTests.size} named tests${passed ? ', all passed against PostGIS' : ' (result verification not requested)'}`);
