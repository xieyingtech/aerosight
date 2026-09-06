import { createHash } from 'node:crypto';
import { readdir, readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve, relative } from 'node:path';
import { createRequire } from 'node:module';

const root = resolve(import.meta.dirname, '..');
const require = createRequire(resolve(root, 'apps/web/package.json'));
const ts = require('typescript');
const app = resolve(root, 'apps/web/app');
const output = resolve(root, 'contracts/go-migration');
const methods = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']);
async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  return (await Promise.all(entries.map(e => e.isDirectory() ? walk(resolve(dir, e.name)) : resolve(dir, e.name)))).flat().sort();
}
const endpoints = {
  requireUser: 'GET /api/auth/session', requireAdmin: 'GET /api/auth/session', auth: 'GET /api/auth/session',
  requireCurrentProjectPermission: 'GET /api/projects/:id (permissions; enforcement remains in each protected API)',
  listTeams: 'GET /api/teams', listManagedTeams: 'GET /api/teams?scope=managed', getTeam: 'GET /api/teams/:id',
  listProjects: 'GET /api/projects', getProject: 'GET /api/projects/:id',
  getProfileData: 'GET /api/profile', getAdminOverview: 'GET /api/admin/overview',
  listAdminUsers: 'GET /api/admin/users', listAdminTeams: 'GET /api/admin/teams', listAdminProjects: 'GET /api/admin/projects',
  listProjectItems: 'GET /api/projects/:id/:kind', getProjectItem: 'GET /api/projects/:id/:kind/:resourceId',
  listIssues: 'GET /api/projects/:id/issues', readIssue: 'GET /api/projects/:id/issues/:issueId',
  listMissionRuns: 'GET /api/projects/:id/task-runs', readMissionWorkbench: 'GET /api/projects/:id/task-runs/:runId',
  listDeviceAdapters: 'GET /api/projects/:id/device-adapters', readProjectDeviceTree: 'GET /api/projects/:id/device-tree',
  listFlightHubConnections: 'GET /api/projects/:id/connectors/dji-flighthub',
  listFlightHubDiscoveryActivity: 'GET /api/projects/:id/connectors/dji-flighthub/activity',
  parseFlightHubWebConfig: 'GET /api/projects/:id/features',
  listAlgorithmCatalog: 'GET /api/projects/:id/algorithm-definitions', listAlgorithmProviders: 'GET /api/projects/:id/algorithm-providers',
  listAlgorithmRuns: 'GET /api/projects/:id/algorithm-runs', readAlgorithmRun: 'GET /api/projects/:id/algorithm-runs/:runId',
  readProjectSituationSnapshot: 'GET /api/projects/:id/snapshot', readPerceptionEvent: 'GET /api/projects/:id/events/:eventId',
  listAIProviders: 'GET /api/admin/ai-providers', listAgentSessions: 'GET /api/projects/:id/agent-sessions',
  loginAction: 'POST /api/auth/login', logoutAction: 'POST /api/auth/logout', createTeamAction: 'POST /api/teams', createProjectAction: 'POST /api/projects',
};
const records = [];
for (const path of await walk(app)) {
  if (!/(route\.ts|page\.tsx|layout\.tsx|actions\.ts)$/.test(path)) continue;
  const source = await readFile(path, 'utf8');
  const ast = ts.createSourceFile(path, source, ts.ScriptTarget.Latest, true, path.endsWith('tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const imports = new Map();
  for (const stmt of ast.statements) if (ts.isImportDeclaration(stmt)) {
    for (const e of stmt.importClause?.namedBindings?.elements ?? []) imports.set(e.name.text, { name: e.propertyName?.text ?? e.name.text, from: stmt.moduleSpecifier.text });
  }
  const calls = new Set();
  function visit(node) {
    if (ts.isCallExpression(node) && ts.isIdentifier(node.expression)) calls.add(node.expression.text);
    ts.forEachChild(node, visit);
  }
  visit(ast);
  const entry = relative(root, path).replaceAll('\\', '/');
  const url = '/' + relative(app, path).replaceAll('\\', '/').replace(/(?:^|\/)\([^/]+\)/g, '').replace(/\/(?:route\.ts|page\.tsx|layout\.tsx)$/, '').replace(/^(?:page|layout)\.tsx$/, '').replace(/\[([^\]]+)\]/g, ':$1').replace(/^\//, '');
  const serviceCalls = [...calls].filter(n => imports.has(n) && (imports.get(n).from.startsWith('@/lib/') || imports.get(n).from === '@/auth'));
  if (path.endsWith('route.ts')) {
    const exported = [];
    for (const stmt of ast.statements) {
      if (ts.isFunctionDeclaration(stmt) && methods.has(stmt.name?.text) && stmt.modifiers?.some(m => m.kind === ts.SyntaxKind.ExportKeyword)) exported.push(stmt.name.text);
      if (ts.isVariableStatement(stmt) && stmt.modifiers?.some(m => m.kind === ts.SyntaxKind.ExportKeyword)) for (const dec of stmt.declarationList.declarations) if (ts.isObjectBindingPattern(dec.name)) for (const item of dec.name.elements) if (methods.has(item.name.text)) exported.push(item.name.text);
    }
    if (!exported.length) throw new Error(`No methods parsed: ${entry}`);
    for (const method of exported) records.push({ kind: 'route', source: entry, method, target: url.startsWith('/api/auth/') ? `${method} /api/auth/* (replaced by explicit auth endpoints)` : `${method} ${url}`, dependencies: serviceCalls.map(n => imports.get(n)), sourceHash: createHash('sha256').update(source).digest('hex'), originalHandler: source });
  } else if (path.endsWith('actions.ts')) {
    for (const stmt of ast.statements) if (ts.isFunctionDeclaration(stmt) && stmt.modifiers?.some(m => m.kind === ts.SyntaxKind.ExportKeyword)) {
      const name = stmt.name.text;
      if (!endpoints[name]) throw new Error(`Unmapped action: ${name}`);
      records.push({ kind: 'action', source: entry, name, target: endpoints[name], originalHandler: stmt.getText(ast) });
    }
  } else {
    const mappings = serviceCalls.map(n => ({ ...imports.get(n), target: endpoints[n] ?? 'client-only helper' }));
    records.push({ kind: 'page', source: entry, url, queries: mappings });
  }
}
await mkdir(output, { recursive: true });
await writeFile(resolve(output, 'entrypoints.json'), JSON.stringify({ version: 1, records }, null, 2) + '\n');
const rows = records.map(r => `| ${r.kind} | ${r.source}${r.name ? '#' + r.name : ''} | ${(r.target ?? r.queries.map(q => q.name + ' → ' + q.target).join('<br>')) || '纯页面/客户端导航'} |`);
await writeFile(resolve(output, 'README.md'), '# Go 迁移接口基线\n\n本清单在迁移前由 TypeScript AST 提取。entrypoints.json 保存原 Route Handler 与 Server Action，作为状态码、响应字段和错误映射的可审查基线；页面条目列出直接服务依赖。测试完成前不得把迁移清单当作实现完成证据。\n\n新增 API 统一执行 design 中的会话、CSRF、作用域和事务要求。各领域权限/幂等/审计基线由原导入的 service 及现有测试定义；迁移时逐项补充 Go 契约测试。\n\n| 类型 | 原入口 | 目标接口/页面查询 |\n| --- | --- | --- |\n' + rows.join('\n') + '\n');
console.log(JSON.stringify({ routes: records.filter(r => r.kind === 'route').length, actions: records.filter(r => r.kind === 'action').length, pagesAndLayouts: records.filter(r => r.kind === 'page').length }));
