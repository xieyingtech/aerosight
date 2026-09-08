import assert from 'node:assert/strict';

export async function verifyProjectWorkspaces(page, detailURL) {
  const project = new URL(detailURL);
  const projectId = project.searchParams.get('projectId');
  const cases = [
    ['tasks', '任务编排'], ['settings', '项目设置'], ['realtime', '实时作业'],
    ['assets', '素材库'], ['issues', '案件'], ['events', '案件'],
    ['devices', '设备管理'], ['algorithms', '算法运行'],
    ['connectors', '连接器'], ['agents', '时空智能体'],
  ];
  const failures = [];
  const responses = new Set();
  const observe = response => {
    const url = new URL(response.url());
    if (!url.pathname.startsWith(`/api/projects/${projectId}`)) return;
    if (response.status() >= 400) failures.push(`${response.status()} ${url.pathname}`);
    else responses.add(url.pathname);
  };
  page.on('response', observe);
  const style = 'https://demotiles.maplibre.org/style.json';
  await page.route(style, route => route.fulfill({ json: { version: 8, sources: {}, layers: [{ id: 'background', type: 'background', paint: { 'background-color': '#dbeafe' } }] } }));
  const verified = [];
  try {
    for (const [section, title] of cases) {
      const target = new URL(`/projects/${section}/`, project.origin);
      target.searchParams.set('projectId', projectId);
      target.searchParams.set('label', '构建后新增项目');
      await page.goto(target.href);
      for (const reload of [false, true]) {
        if (reload) await page.reload();
        await page.getByRole('heading', { name: title, exact: true }).waitFor({ state: 'visible' });
        if (section === 'tasks') await page.getByRole('heading', { name: '任务运行', exact: true }).waitFor({ state: 'visible' });
        const current = new URL(page.url());
        assert.equal(current.searchParams.get('projectId'), projectId);
        assert.equal(current.searchParams.get('label'), '构建后新增项目');
        assert.equal(current.pathname, `/projects/${section === 'events' ? 'issues' : section}/`);
        assert.deepEqual(await page.evaluate(() => window.cspViolations), [], `${section} CSP`);
      }
      verified.push({ section, title, directVisit: true, reload: true });
    }
    assert.deepEqual(failures, [], 'workspace API failures');
    return { projectId, verified, responses: [...responses].sort() };
  } finally {
    page.off('response', observe);
    await page.unroute(style);
    await page.goto(detailURL);
  }
}
