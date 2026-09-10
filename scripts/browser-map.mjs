import assert from 'node:assert/strict';
import { resolve } from 'node:path';

export async function verifyMap(page, detailURL, output, seedSQL) {
  const projectId=Number(new URL(detailURL).searchParams.get('projectId'));
  assert(Number.isSafeInteger(projectId) && projectId>0);
  seedSQL(`with adapter as (
    insert into device_adapters(project_id,team_id,name,adapter_type)
    select id,team_id,'CSP map adapter','simulator' from projects where id=${projectId} returning id,project_id,team_id
  ), device as (
    insert into devices(project_id,name,type,adapter_id,device_type_id,status,last_seen_at)
    select adapter.project_id,'CSP map device','drone',adapter.id,dt.id,'online',now()
    from adapter cross join device_types dt where dt.type_key='legacy.device' and dt.version=1 returning id,adapter_id,project_id
  ), observation as (
    insert into observations(project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at)
    select device.project_id,adapter.team_id,adapter.id,device.id,'pose','csp-map-pose',now(),now() from device join adapter on adapter.id=device.adapter_id
    returning id,project_id,device_id
  ) insert into poses(observation_id,project_id,device_id,captured_at,standard_position)
    select id,project_id,device_id,now(),ST_SetSRID(ST_MakePoint(116.397,39.908,50),4326) from observation;`);
  // Keep the browser's external-origin CSP check, while making the style bytes
  // deterministic and independent of the public demo tile service's uptime.
  let styleRequests=0;
  await page.route('https://tiles.openfreemap.org/styles/liberty',route=>{
    styleRequests++;
    return route.fulfill({contentType:'application/json',body:JSON.stringify({version:8,sources:{},layers:[{id:'acceptance-background',type:'background',paint:{'background-color':'#dbeafe'}}]})});
  });
  const workers=[];
  const onWorker=worker=>workers.push(worker.url());
  page.on('worker',onWorker);
  try {
    await page.goto(detailURL);
    const canvas=page.locator('.maplibregl-canvas');
    await canvas.waitFor({state:'visible'});
    const selected=page.getByRole('heading',{name:'CSP map device',exact:true});
    for(let attempt=0;attempt<15;attempt++) {
      await canvas.click();
      try { await selected.waitFor({state:'visible',timeout:1000});break; } catch { if(attempt===14)throw new Error('GeoJSON device was not rendered/selectable'); }
    }
    assert(styleRequests>0,'map style never loaded');
    assert(workers.some(url=>url.startsWith('blob:')),'MapLibre blob worker did not start');
    assert.deepEqual(await page.evaluate(()=>window.cspViolations),[],'map CSP blocked resources');
    await page.waitForFunction(()=>{
      const canvas=document.querySelector('.maplibregl-canvas');
      return canvas && Math.abs(canvas.width-canvas.getBoundingClientRect().width*devicePixelRatio)<2;
    });
    await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
    await page.screenshot({path:resolve(output,'map-selected.png'),fullPage:true});
    await page.setViewportSize({width:1400,height:900});
    await page.setViewportSize({width:1280,height:900});
    await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
    await page.screenshot({path:resolve(output,'map-resized.png'),fullPage:true});
    const dimensions=await page.locator('.maplibregl-canvas').evaluate(canvas=>({width:canvas.width,css:canvas.getBoundingClientRect().width,map:canvas.closest('.maplibregl-map').getBoundingClientRect().width,parent:canvas.closest('.maplibregl-map').parentElement.getBoundingClientRect().width}));
    return {styleRequests,blobWorkers:workers.filter(url=>url.startsWith('blob:')).length,deviceRenderedAndSelected:true,externalStyle:'controlled fixture',dimensions};
  } finally {
    page.off('worker',onWorker);
    await page.unroute('https://tiles.openfreemap.org/styles/liberty');
  }
}
