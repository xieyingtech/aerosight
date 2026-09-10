import assert from 'node:assert/strict';

export async function verifyLegacyLinks(page, context, origin, detailURL) {
  const uuid='01234567-89ab-cdef-0123-456789abcdef';
  const cases=[
    ['/teams/42','/teams/detail/',{teamId:'42'}],
    ['/projects/42','/projects/detail/',{projectId:'42'}],
    ...['tasks','settings','realtime','assets','issues','events','devices','algorithms','connectors','agents'].map(section=>[`/projects/42/${section}`,`/projects/${section}/`,{projectId:'42'}]),
    ['/projects/42/tasks/runs/56','/projects/tasks/runs/detail/',{projectId:'42',runId:'56'}],
    [`/projects/42/algorithms/runs/${uuid}`,'/projects/algorithms/runs/detail/',{projectId:'42',runId:uuid}],
    ['/projects/42/issues/7','/projects/issues/detail/',{projectId:'42',issueId:'7'}],
    [`/projects/42/events/${uuid}`,'/projects/events/detail/',{projectId:'42',eventId:uuid}],
  ];
  for(const [old,path,ids] of cases) {
    for(const method of ['GET','HEAD']) {
      const query=new URLSearchParams({label:'中文&x'});
      query.append('layer','one');query.append('layer','two');
      for(const key of Object.keys(ids)){query.append(key,'99');query.append(key,'100');}
      const response=await context.request.fetch(origin+old+'?'+query,{method,maxRedirects:0});
      assert.equal(response.status(),307,`${method} ${old}`);
      const target=new URL(response.headers().location,origin);
      assert.equal(target.pathname,path);
      for(const [key,value] of Object.entries(ids))assert.deepEqual(target.searchParams.getAll(key),[value],`${old} conflicting ${key}`);
      assert.deepEqual(target.searchParams.getAll('layer'),['one','two']);
      assert.equal(target.searchParams.get('label'),'中文&x');
    }
  }
  for(const path of ['/projects/0','/projects/01','/projects/2147483648','/projects/1/tasks/runs/0','/projects/1/events/not-uuid']) {
    const response=await context.request.get(origin+path,{maxRedirects:0});
    assert.equal(response.status(),404,`invalid legacy path ${path}`);
  }
  const projectId=new URL(detailURL).searchParams.get('projectId');
  await page.goto(origin+'/projects/');
  await page.getByRole('heading',{name:'项目',exact:true}).waitFor({state:'visible'});
  await page.goto(`${origin}/projects/${projectId}?projectId=999&label=retained`);
  await page.getByRole('heading',{name:'Browser acceptance project',exact:true}).waitFor({state:'visible'});
  assert.equal(new URL(page.url()).searchParams.get('projectId'),projectId);
  assert.equal(new URL(page.url()).searchParams.get('label'),'retained');
  await page.goBack();
  await page.getByRole('heading',{name:'项目',exact:true}).waitFor({state:'visible'});
  await page.goForward();
  await page.getByRole('heading',{name:'Browser acceptance project',exact:true}).waitFor({state:'visible'});
  assert.equal(new URL(page.url()).searchParams.get('label'),'retained');
  return {mappings:cases.length,methods:['GET','HEAD'],invalidPaths:5,history:true};
}
