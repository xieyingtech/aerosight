import assert from 'node:assert/strict';

export async function verifyPageStates(page, origin) {
  const session = url => url.pathname === '/api/auth/session';
  await page.route(session, route => route.fulfill({status:503,contentType:'application/json',body:'{"error":"ACCEPTANCE_UNAVAILABLE"}'}));
  await page.goto(origin+'/projects/');
  await page.getByRole('alert').filter({hasText:'暂时无法获取登录状态。'}).waitFor({state:'visible'});
  await page.unroute(session);
  await page.getByRole('button',{name:'重试',exact:true}).click();
  await page.getByRole('heading',{name:'项目',exact:true}).waitFor({state:'visible'});
  const pages = [
    ['/projects/','/api/projects','项目'], ['/teams/','/api/teams','团队'],
    ['/profile/','/api/profile','个人中心'], ['/admin/','/api/admin/overview','管理总览'],
    ['/admin/users/','/api/admin/users','用户管理'], ['/admin/teams/','/api/admin/teams','团队管理'],
    ['/admin/projects/','/api/admin/projects','项目管理'], ['/admin/ai-providers/','/api/admin/ai-providers','AI Provider'],
  ];
  for (const [path, api, heading] of pages) {
    const matches = url => url.pathname === api;
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    await page.route(matches, async route => {
      await gate;
      await route.fulfill({status:503,contentType:'application/json',body:'{"error":"ACCEPTANCE_UNAVAILABLE"}'});
    });
    try {
      await page.goto(origin+path);
      await page.getByRole('status').filter({hasText:'正在加载'}).waitFor({state:'visible'});
    } finally { release(); }
    await page.getByRole('alert').filter({hasText:'加载失败，请重试。'}).waitFor({state:'visible'});
    await page.unroute(matches);
    await page.getByRole('button',{name:'重试',exact:true}).click();
    await page.getByRole('heading',{name:heading,exact:true}).waitFor({state:'visible'});
    await page.route(matches, route => route.fulfill({status:403,contentType:'application/json',body:'{"error":"FORBIDDEN"}'}));
    await page.reload();
    await page.getByRole('alert').filter({hasText:'你没有访问此内容的权限。'}).waitFor({state:'visible'});
    assert.equal(await page.getByRole('heading',{name:heading,exact:true}).count(),0,`${path} rendered protected data after denial`);
    await page.unroute(matches);
  }
  await page.goto(origin+'/projects/');
  await page.getByRole('heading',{name:'项目',exact:true}).waitFor({state:'visible'});
  await page.getByText('暂无数据',{exact:true}).waitFor({state:'visible'});
  return pages.map(([path])=>path);
}
