import assert from 'node:assert/strict';
import {verifyRealInspectionReview} from './inspection-review-browser-fixture.mjs';
import { randomBytes, createHash } from 'node:crypto';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { existsSync, mkdirSync, openSync, closeSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { resolve } from 'node:path';
import pg from 'pg';
import { chromium } from 'playwright';

const root=resolve(import.meta.dirname,'..');
if(existsSync(resolve(root,'.env.local'))) process.loadEnvFile(resolve(root,'.env.local'));
assert(process.env.DATABASE_URL,'Configure DATABASE_URL for a server allowing disposable test databases');
const name='aerosight_test_author_'+randomBytes(8).toString('hex');
const output=resolve(root,'.build',name);mkdirSync(output,{recursive:true});
const log=openSync(resolve(output,'server.log'),'w');
const admin=new pg.Client({connectionString:process.env.DATABASE_URL});
let app,browser,created=false;
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
try{
 await admin.connect();await admin.query(`CREATE DATABASE "${name}"`);created=true;
 const url=new URL(process.env.DATABASE_URL);url.pathname='/'+name;
 const probe=createServer();await new Promise(resolve=>probe.listen(0,'127.0.0.1',resolve));const port=probe.address().port;await new Promise(resolve=>probe.close(resolve));
 const origin=`http://127.0.0.1:${port}`;
 const appEnv={...process.env,
  DATABASE_URL:url.toString(),AEROSIGHT_ENV:'development',PORT:String(port),PUBLIC_ORIGIN:origin,HOST:'127.0.0.1',
  APP_SECRET:randomBytes(32).toString('hex'),CSRF_SECRET:randomBytes(32).toString('base64'),DATA_DIR:resolve(output,'objects'),
  GIN_MODE:'release',DJI_FLIGHTHUB_ENABLED:'false',CALLBACK_PUBLIC_BASE_URL:'',MEDIA_API_BASE_URL:'',MEDIA_ADMIN_USER:'',MEDIA_ADMIN_PASSWORD:''};
 const startApp=async()=>{
  app=spawn(resolve(root,'.build/aerosight'),['serve'],{cwd:output,stdio:['ignore',log,log],env:appEnv});
  let ready=false;
  for(let i=0;i<120;i++){
   assert.equal(app.exitCode,null,'Test app exited; inspect server.log');
   assert.equal(app.signalCode,null,'Test app terminated; inspect server.log');
   try{ready=(await fetch(origin+'/api/auth/csrf',{signal:AbortSignal.timeout(1000)})).ok;}catch{}
   if(ready)break;await delay(250);
  }
  assert(ready,'Test app readiness timed out');
 };
 const restarts=[];
 const restartServer=async boundary=>{
  const previous=app;
  const exited=once(previous,'exit');
  assert(previous.kill('SIGKILL'),'Disposable server kill failed');
  const [code,signal]=await exited;
  assert.equal(code,null);assert.equal(signal,'SIGKILL');
  await startApp();
  restarts.push({boundary,previousPid:previous.pid,currentPid:app.pid,signal});
 };
 await startApp();
 browser=await chromium.launch({headless:true,...process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}:{}});
 const context=await browser.newContext();const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(origin+'/login/');
 await page.getByLabel('邮箱或手机号').fill('admin@example.com');await page.getByLabel('密码',{exact:true}).fill('admin');await page.getByRole('button',{name:'登录',exact:true}).click();
 await page.waitForURL(url=>/^\/projects\/?$/.test(url.pathname));
 const post=async(path,body,method='POST')=>page.evaluate(async({path,body,method})=>{const token=await(await fetch('/api/auth/csrf')).json();const res=await fetch(path,{method,headers:{'Content-Type':'application/json','X-CSRF-Token':token.csrfToken},body:JSON.stringify(body)});return {status:res.status,body:await res.json()};},{path,body,method});
 const team=await post('/api/teams',{name:'Inspection authoring test'});assert.equal(team.status,201,JSON.stringify(team));
 const project=await post('/api/projects',{teamId:team.body.id,name:'Inspection authoring project'});assert.equal(project.status,201,JSON.stringify(project));
 const pid=project.body.id;
 const source='# 保留说明\napiVersion: aerosight/v2\nname: 原始任务\ntrigger: {type: manual}\nsteps:\n  - key: report\n    uses: report.generate\n    with:\n      custom: {nested: [1, 2]}\n';
 const task=await post(`/api/projects/${pid}/tasks`,{sourceFormat:'yaml',source,idempotencyKey:'browser-author-create'});assert.equal(task.status,201,JSON.stringify(task));
 await page.goto(`${origin}/projects/tasks/detail/?projectId=${pid}&taskId=${task.body.taskId}`);
 const editor=page.getByLabel('任务定义 YAML');await editor.waitFor();assert.equal(await editor.inputValue(),source);
 await page.getByRole('button',{name:'参数表单',exact:true}).click();await page.getByLabel('任务名称',{exact:true}).fill('表单修改任务');await page.getByLabel('触发方式').selectOption('schedule');await page.getByLabel('Cron 表达式').fill('0 9 * * *');await page.getByRole('button',{name:'原始编辑',exact:true}).click();
 assert.match(await editor.inputValue(),/0 9 \* \* \*/);
 assert.match(await editor.inputValue(),/name: 表单修改任务/);assert.match(await editor.inputValue(),/custom: \{ nested: \[ 1, 2 \] \}|custom: \{nested: \[1, 2\]\}/);
 await page.getByLabel('定义格式').selectOption('json');const json=JSON.parse(await page.getByLabel('任务定义 JSON').inputValue());assert.deepEqual(json.steps[0].with.custom,{nested:[1,2]});
 await page.getByLabel('定义格式').selectOption('yaml');
 const valid=await editor.inputValue();await editor.fill('name: [unfinished');await page.getByLabel('定义格式').selectOption('json');assert.equal(await editor.inputValue(),'name: [unfinished');assert(await page.getByRole('alert').count()>0);
 await editor.fill(valid);
 const saved=page.waitForResponse(r=>r.url().endsWith('/versions')&&r.request().method()==='POST');await page.getByRole('button',{name:'保存草稿',exact:true}).click();assert.equal((await saved).status(),200);
 await page.reload();await editor.waitFor();assert.match(await editor.inputValue(),/表单修改任务/);
 await page.screenshot({path:resolve(output,'authoring.png'),fullPage:true});
 await page.goto(`${origin}/projects/tasks/?projectId=${pid}`);
 await page.getByRole('button',{name:'新建任务',exact:true}).click();
 const readiness=page.getByRole('region',{name:'巡检资源就绪检查'});
 await readiness.getByText('图片模板需先准备授权图片',{exact:false}).waitFor();
 await readiness.getByRole('button',{name:'刷新资源',exact:true}).click();
 await readiness.getByText('外部检测暂缺可用配置',{exact:false}).waitFor();
 await readiness.getByRole('link',{name:'查看智能体',exact:true}).waitFor();

 await page.getByRole('button',{name:'参数表单',exact:true}).click();
 await page.getByLabel('assetIds',{exact:true}).fill('[1, 2]');
 await page.getByLabel('algorithmDefinitionVersionId',{exact:true}).fill('9');
 await page.getByLabel('temperature',{exact:true}).fill('0.7');
 await page.getByRole('button',{name:'原始编辑',exact:true}).click();
 const createdThroughForm=page.waitForResponse(r=>r.url().endsWith(`/api/projects/${pid}/tasks`)&&r.request().method()==='POST');
 await page.getByRole('button',{name:'创建任务草稿',exact:true}).click();

 const formResponse=await createdThroughForm;
 assert.equal(formResponse.status(),201,await formResponse.text());
 await page.waitForURL(url=>url.pathname.includes('/tasks/detail/'));
 await page.getByLabel('任务定义 YAML').waitFor();
 let formSource=await page.getByLabel('任务定义 YAML').inputValue();
 assert.match(formSource,/algorithmDefinitionVersionId: 9/);
 assert.match(formSource,/assetIds:/);
 assert.match(formSource,/temperature: 0.7/);
 await page.getByLabel('任务定义 YAML').fill(formSource.replace('temperature: 0.7','temperature: 0.4'));
 await page.getByRole('button',{name:'参数表单',exact:true}).click();
 assert.equal(await page.getByLabel('temperature',{exact:true}).inputValue(),'0.4');
 await page.getByRole('button',{name:'原始编辑',exact:true}).click();
 const savedTemperature=page.waitForResponse(r=>r.url().includes('/versions')&&r.request().method()==='POST');
 await page.getByRole('button',{name:'保存草稿',exact:true}).click();
 assert.equal((await savedTemperature).status(),200);
 await page.reload();
 await page.getByLabel('任务定义 YAML').waitFor();
 formSource=await page.getByLabel('任务定义 YAML').inputValue();
 assert.match(formSource,/temperature: 0.4/);

 const validationSource='apiVersion: aerosight/v2\nname: 校验样例\ntrigger: {type: manual}\nsteps: [{key: report, uses: report.generate, with: {scope: all-projects}}]\n';
 await page.getByLabel('任务定义 YAML').fill(validationSource);
 await page.getByRole('button',{name:'校验当前草稿',exact:true}).click();
 await page.getByText('/with/scope · const',{exact:true}).waitFor();
 await page.getByLabel('任务定义 YAML').fill(validationSource.replace('all-projects','current-run'));
 assert.equal(await page.getByText('/with/scope · const',{exact:true}).count(),0);
 await page.getByRole('button',{name:'校验当前草稿',exact:true}).click();
 await page.getByText('当前草稿校验通过；发布和运行前仍会重新检查权限与资源。',{exact:true}).waitFor();
 await page.getByLabel('任务定义 YAML').fill(formSource);
 assert.equal(await page.getByText('当前草稿校验通过；发布和运行前仍会重新检查权限与资源。',{exact:true}).count(),0);


 // Exercise the actual background consumer without any device or physical action.
 const reportTask=await post(`/api/projects/${pid}/tasks`,{sourceFormat:'yaml',source:'apiVersion: aerosight/v2\nname: 无设备报告运行\ntrigger: {type: manual}\nsteps: [{key: report, uses: report.generate}]\n',idempotencyKey:'runtime-report-task'});
 assert.equal(reportTask.status,201,JSON.stringify(reportTask));
 const reportPath=`/api/projects/${pid}/tasks/${reportTask.body.taskId}`;
 const publication=await post(reportPath+'/versions',{action:'publish',versionId:reportTask.body.versionId,expectedRevision:1});assert.equal(publication.status,200,JSON.stringify(publication));
 const activation=await post(reportPath,{status:'active'},'PATCH');assert.equal(activation.status,200,JSON.stringify(activation));
 const invocation={type:'manual',idempotencyKey:'runtime-report-run',occurredAt:new Date().toISOString(),inputs:{}};
 const run=await post(reportPath+'/runs',invocation);assert.equal(run.status,201,JSON.stringify(run));
 let runModel;
 for(let i=0;i<120;i++){
  runModel=await page.evaluate(async path=>{const res=await fetch(path);if(!res.ok)throw new Error(await res.text());return res.json();},`/api/projects/${pid}/task-runs/${run.body.taskRunId}`);
  if(['succeeded','failed','paused'].includes(runModel.run.status))break;
  await delay(250);
 }
 assert.equal(runModel.run.status,'succeeded',JSON.stringify(runModel.run));
 assert.equal(runModel.steps[0].status,'succeeded');
 assert(runModel.steps[0].outputSnapshot.reportId,'Missing validated report output');
 const runReplay=await post(reportPath+'/runs',invocation);assert.equal(runReplay.body.taskRunId,run.body.taskRunId);
 // Simulate a lost browser response after the real server commits a manual Run.
 let lostRun,manualBodies=[];
 await page.route(`**${reportPath}/runs`,async route=>{
  manualBodies.push(route.request().postDataJSON());
  if(manualBodies.length===1){const committed=await route.fetch();assert.equal(committed.status(),201);lostRun=(await committed.json()).taskRunId;await route.abort('failed');}
  else await route.continue();
 });
 await page.goto(`${origin}/projects/tasks/detail/?projectId=${pid}&taskId=${reportTask.body.taskId}`);
 await page.getByText(/当前执行委托者：.*（#\d+）/).waitFor();
 await page.getByRole('button',{name:'手动运行',exact:true}).click();
 await page.getByText('Failed to fetch',{exact:true}).waitFor();
 const retried=page.waitForResponse(r=>r.url().endsWith(reportPath+'/runs')&&r.request().method()==='POST');
 await page.getByRole('button',{name:'手动运行',exact:true}).click();
 assert.equal((await retried).status(),200);
 await page.waitForURL(url=>url.pathname.includes('/tasks/runs/detail/')&&url.searchParams.get('runId')===String(lostRun));
 assert.equal(manualBodies.length,2);assert.deepEqual(manualBodies[1],manualBodies[0]);
 await page.unroute(`**${reportPath}/runs`);

 // Seed only a disposable local connector for policy UI acceptance; no remote access.
 const fixture=new pg.Client({connectionString:url.toString()});await fixture.connect();
 let observationRun;
 let policyConnector;
 try{
  // A readable PNG fixture exercises storage and immutable provenance, not aerial/model acceptance.
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII=','base64');
  const key=`projects/${pid}/inspection-fixture.png`;
  mkdirSync(resolve(output,'objects',`projects/${pid}`),{recursive:true});
  writeFileSync(resolve(output,'objects',key),png);
  const checksum=createHash('sha256').update(png).digest('hex');
  const asset=await fixture.query(`insert into assets(project_id,team_id,task_run_id,kind,storage_key,logical_key,checksum_sha256) values($1,$2,$3,'image',$4,'inspection-fixture',$5) returning id`,[pid,team.body.id,run.body.taskRunId,key,checksum]);
  const assetId=asset.rows[0].id;
  const simulatedAdapter=(await fixture.query("insert into device_adapters(project_id,team_id,name,adapter_type) values($1,$2,'图片来源模拟设备','simulator') returning id",[pid,team.body.id])).rows[0].id;
  const simulatedDevice=(await fixture.query("insert into devices(project_id,name,type,adapter_id,device_type_id) select $1,'图片来源模拟设备','drone',$2,id from device_types where type_key='legacy.device' returning id",[pid,simulatedAdapter])).rows[0].id;
  await fixture.query('update assets set device_id=$2 where id=$1',[assetId,simulatedDevice]);
  const observationTask=await post(`/api/projects/${pid}/tasks`,{sourceFormat:'json',source:JSON.stringify({apiVersion:'aerosight/v2',name:'图片观察运行',trigger:{type:'manual'},steps:[{key:'observe',uses:'inspection.observe',with:{mode:'assets',assetIds:[assetId]}},{key:'report',uses:'report.generate'}]}),idempotencyKey:'runtime-observation-task'});
  assert.equal(observationTask.status,201,JSON.stringify(observationTask));
  const path=`/api/projects/${pid}/tasks/${observationTask.body.taskId}`;
  const published=await post(path+'/versions',{action:'publish',versionId:observationTask.body.versionId,expectedRevision:1});assert.equal(published.status,200,JSON.stringify(published));
  assert.equal((await post(path,{status:'active'},'PATCH')).status,200);
  const input={type:'manual',idempotencyKey:'runtime-observation-run',occurredAt:new Date().toISOString(),inputs:{}};
  observationRun=await post(path+'/runs',input);assert.equal(observationRun.status,201,JSON.stringify(observationRun));
  let model;
  for(let i=0;i<120;i++){
   model=await page.evaluate(async path=>{const res=await fetch(path);if(!res.ok)throw new Error(await res.text());return res.json();},`/api/projects/${pid}/task-runs/${observationRun.body.taskRunId}`);
   if(['succeeded','failed','paused'].includes(model.run.status))break;
   await delay(250);
  }
  assert.equal(model.run.status,'succeeded',JSON.stringify(model));
  assert.equal(model.steps.length,2);
  const observationId=model.steps[0].outputSnapshot.observationId;
  assert(observationId);
  const manifest=await page.evaluate(async path=>{const res=await fetch(path);if(!res.ok)throw new Error(await res.text());return res.json();},`/api/projects/${pid}/inspection/observations/${observationId}`);
  assert.equal(manifest.completeness,'complete');assert.equal(manifest.assets.length,1);
  assert.equal(manifest.assets[0].checksumSha256,checksum);assert.equal(manifest.assets[0].sourceRunId,run.body.taskRunId);
  assert.equal(manifest.assets[0].sourceDeviceMode,'simulator');
  assert.equal(manifest.assets[0].version,1);assert(model.steps[1].outputSnapshot.reportId);
  assert.equal((await post(path+'/runs',input)).body.taskRunId,observationRun.body.taskRunId);
  const provenance=await fixture.query('select task_run_id from assets where id=$1',[assetId]);assert.equal(provenance.rows[0].task_run_id,run.body.taskRunId);
  await page.goto(`${origin}/projects/tasks/runs/detail/?projectId=${pid}&runId=${observationRun.body.taskRunId}`);
  await page.getByRole('link',{name:'查看巡检进展与待复核摘要',exact:true}).click();
  await page.getByText('尚无研判记录，不能据此认定没有异常。',{exact:true}).waitFor();
  await page.getByRole('button',{name:'刷新摘要',exact:true}).click();
  await page.getByText('尚无研判记录，不能据此认定没有异常。',{exact:true}).waitFor();

  await page.goto(`${origin}/projects/inspection/observation/?projectId=${pid}&observationId=${observationId}`);
  const originalImage=page.getByRole('img',{name:`观察原图 ${assetId}`,exact:true});
  await originalImage.waitFor();
  await page.getByText('关联模拟设备，不作为实飞证明',{exact:true}).waitFor();
  await page.waitForFunction(()=>{const image=document.querySelector('img[alt^="观察原图"]');return image?.complete&&image.naturalWidth>0;});
  await page.screenshot({path:resolve(output,'observation-preview.png'),fullPage:true});
  const observationRoute=`**/api/projects/${pid}/inspection/observations/${observationId}`;
  await page.route(observationRoute,route=>route.abort('internetdisconnected'));
  await page.getByRole('button',{name:'刷新观察与原图',exact:true}).click();
  await page.getByText('加载失败，请重试。',{exact:true}).waitFor();
  assert.equal(await page.getByRole('img',{name:`观察原图 ${assetId}`,exact:true}).count(),0);
  await page.unroute(observationRoute);
  await page.getByRole('button',{name:'重试',exact:true}).click();
  await page.waitForFunction(()=>{const image=document.querySelector('img[alt^="观察原图"]');return image?.complete&&image.naturalWidth>0;});

  writeFileSync(resolve(output,'objects',key),Buffer.from('changed fixture bytes'));
  await page.reload();
  await page.getByRole('alert').filter({hasText:'原图不可用、版本发生变化或无法验证'}).waitFor();
  writeFileSync(resolve(output,'objects',key),png);
  await page.goto(`${origin}/projects/reports/detail/?projectId=${pid}&reportId=${model.steps[1].outputSnapshot.reportId}`);
  await page.getByText('本次分析范围',{exact:true}).waitFor();
  await page.getByText('关联案件（0）',{exact:true}).waitFor();


  const row=await fixture.query(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json)
   select $1,$2,'巡检策略测试','dji-flighthub2',id,'2','connected','{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"巡检策略测试"}'
   from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0' returning id`,[pid,team.body.id]);
  policyConnector=row.rows[0].id;
  const flightDevice=(await fixture.query(`insert into devices(project_id,name,type,adapter_id,device_type_id) select $1,'航线选择样本设备','drone',$2,id from device_types where type_key='legacy.device' returning id`,[pid,policyConnector])).rows[0].id;
  await fixture.query(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'plan-fixture','managed')`,[pid,team.body.id,policyConnector,flightDevice]);
  const wayline=(await fixture.query(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,summary_json) values($1,$2,$3,'wayline','plan-fixture','1000:200','{"name":"航线选择样本","updatedAt":1000,"sizeBytes":200}') returning id`,[pid,team.body.id,policyConnector])).rows[0].id;
  await page.goto(`${origin}/projects/tasks/?projectId=${pid}`);
  await page.getByRole('button',{name:'新建任务',exact:true}).click();
  await page.getByLabel('巡检模板').selectOption('flighthub-flight');
  await page.getByLabel('司空预设航线').selectOption(String(wayline));
  await page.getByLabel('司空执行设备').selectOption(String(flightDevice));
  await page.getByRole('button',{name:'预览航线版本',exact:true}).click();
  await page.getByRole('button',{name:'将航线版本写入草稿',exact:true}).click();
  assert.match(await page.getByLabel('任务定义 YAML').inputValue(),/remoteVersion:.*1000:200/);
  await page.screenshot({path:resolve(output,'flight-plan-picker.png'),fullPage:true});
  await page.getByLabel('司空执行设备').selectOption('');
  assert.equal(await page.getByRole('button',{name:'将航线版本写入草稿',exact:true}).count(),0);
  assert.equal(Number((await fixture.query('select count(*) from connector_action_jobs where project_id=$1',[pid])).rows[0].count),0);
  const flightSource=await page.getByLabel('任务定义 YAML').inputValue();
  const flightDraftResponse=page.waitForResponse(r=>r.url().endsWith(`/api/projects/${pid}/tasks`)&&r.request().method()==='POST');
  await page.getByRole('button',{name:'创建任务草稿',exact:true}).click();
  const flightDraft=await flightDraftResponse;assert.equal(flightDraft.status(),201,await flightDraft.text());
  await page.waitForURL(url=>url.pathname.includes('/tasks/detail/'));
  assert.equal(await page.getByLabel('任务定义 YAML').inputValue(),flightSource);
  const flightValidation=await post(`/api/projects/${pid}/tasks/validate`,{sourceFormat:'yaml',source:flightSource});
  assert.equal(flightValidation.status,200);assert.equal(flightValidation.body.canPublish,false);
  assert.equal(flightValidation.body.issues[0].code,'TASK_CAPABILITY_NOT_DEPLOYED:inspection.observe');
  await fixture.query(`update connector_remote_resources set remote_version='2000:200',summary_json='{"name":"航线选择样本","updatedAt":2000,"sizeBytes":200}' where id=$1`,[wayline]);
  const stalePlan=await post(`/api/projects/${pid}/tasks/validate`,{sourceFormat:'yaml',source:flightSource});
  assert.equal(stalePlan.body.issues[0].code,'TASK_WAYLINE_VERSION_CHANGED');
  await page.getByLabel('司空预设航线').selectOption(String(wayline));
  await page.getByLabel('司空执行设备').selectOption(String(flightDevice));
  await page.getByRole('button',{name:'预览航线版本',exact:true}).click();
  await page.getByRole('button',{name:'将航线版本写入草稿',exact:true}).click();
  assert.match(await page.getByLabel('任务定义 YAML').inputValue(),/remoteVersion:.*2000:200/);
  const savedPlan=page.waitForResponse(r=>r.url().includes('/versions')&&r.request().method()==='POST');
  await page.getByRole('button',{name:'保存草稿',exact:true}).click();
  assert.equal((await savedPlan).status(),200);
  await page.reload();
  await page.getByLabel('任务定义 YAML').waitFor();
  assert.match(await page.getByLabel('任务定义 YAML').inputValue(),/remoteVersion:.*2000:200/);
  assert.equal(Number((await fixture.query('select count(*) from connector_action_jobs where project_id=$1',[pid])).rows[0].count),0);



  const resource=await fixture.query(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,summary_json)
   values($1,$2,$3,'ai-alert','fixture-alert','{"label":"person","capturedAt":"2026-09-11T08:00:00Z"}') returning id`,[pid,team.body.id,policyConnector]);
  await fixture.query(`insert into inspection_alert_sources(project_id,connector_instance_id,remote_resource_id,remote_flight_id,evidence_json) values($1,$2,$3,'fixture-flight','{}')`,[pid,policyConnector,resource.rows[0].id]);
  await fixture.query(`insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership) values($1,$2,'fixture-flight','pending')`,[pid,policyConnector]);
 }finally{await fixture.end();}
 await page.goto(`${origin}/projects/connectors/?projectId=${pid}`);
 await page.getByRole('button',{name:'管理',exact:true}).click();
 const policy=page.getByRole('region',{name:'巡检告警处理策略'});
 await policy.getByRole('button',{name:'开启 Task 告警管理',exact:true}).click();
 await policy.getByText('Task 管理已开启',{exact:true}).waitFor();
 await page.reload();await page.getByRole('button',{name:'管理',exact:true}).click();
 await policy.getByText('Task 管理已开启',{exact:true}).waitFor();
 await page.screenshot({path:resolve(output,'alert-policy.png'),fullPage:true});
 await policy.getByRole('button',{name:'确认该架次沿用旧建案规则',exact:true}).click();
 await policy.getByText('暂无待确认或已接管的告警证据。',{exact:true}).waitFor();
 await policy.getByRole('button',{name:'关闭新架次接管',exact:true}).click();
 await policy.getByText('新架次默认沿用旧建案规则',{exact:true}).waitFor();
 const reviewDB=new pg.Client({connectionString:url.toString()});await reviewDB.connect();
 try {await verifyRealInspectionReview({page,db:reviewDB,post,pid,teamId:team.body.id,origin,output,restartServer});}finally{await reviewDB.end();}
 // These three routes are explicit UI protocol fixtures. The server review
 // authorization/transaction path has separate real-database HTTP tests.
 const assessmentId='11111111-2222-4333-8444-555555555555',evidenceId='22222222-2222-4333-8444-555555555555';
 let reviewSource='external';
 let reviewState='needs_review',reviewRevision=1,canReview=true,reviewConflict=false;
 const submitted=[];
 const decision={candidateId:'fixture-candidate',action:'needs_review',reason:'需要核实原始影像',evidenceRefs:['fixture-evidence'],missingInformation:['历史影像']};
 await page.route(`**/api/projects/${pid}/inspection/assessments/${assessmentId}`,route=>route.fulfill({json:{id:assessmentId,taskRunId:1,evidenceSetId:evidenceId,status:reviewState,runStatus:reviewState==='needs_review'?'paused':'running',revision:reviewRevision,canReview,modelVersion:'UI protocol fixture',promptVersion:'inspection-assessment-v1',originalOutput:JSON.stringify({decisions:[decision]}),revisions:[{revision:reviewRevision,source:'model',decisions:[decision]}]}}));
 await page.route(`**/api/projects/${pid}/inspection/evidence-sets/${evidenceId}`,route=>route.fulfill({json:{id:evidenceId,observationId:assessmentId,source:reviewSource,completeness:'complete',targetAlgorithmConfirmed:true,candidates:[{id:'fixture-candidate',evidenceRefs:['fixture-evidence'],position:{source:'unknown',quality:'image-only'}}]}}));
 await page.route(`**/api/projects/${pid}/inspection/assessments/${assessmentId}/review`,async route=>{
  if(reviewConflict){await route.fulfill({status:409,json:{error:'INSPECTION_REVIEW_REVISION_CONFLICT'}});return;}
  submitted.push(route.request().postDataJSON());reviewState='succeeded';reviewRevision++;
  await route.fulfill({json:{assessmentId,revision:reviewRevision,replayed:false}});
 });
 await page.route(`**/api/projects/${pid}/task-runs/2147483646/inspection-summary`,route=>route.fulfill({json:{runId:2147483646,runStatus:'paused',stateVersion:2,pendingReviewCount:1,final:false,dataGaps:['缺少历史影像'],inspection:{scopeNotice:'仅限冻结样本',observations:[],evidenceSets:[],assessments:[{id:assessmentId,status:'needs_review',revision:1,decisions:[decision]}]}}}));
 await page.goto(`${origin}/projects/inspection/summary/?projectId=${pid}&runId=2147483646`);
 await page.getByRole('status').filter({hasText:'有 1 项研判待人工复核'}).waitFor();
 await page.getByText('缺少历史影像',{exact:true}).waitFor();
 await page.screenshot({path:resolve(output,'inspection-summary-fixture.png'),fullPage:true});
 await page.getByRole('link',{name:'查看研判 · 修订 1 · needs_review',exact:true}).click();
 const reviewURL=`${origin}/projects/inspection/assessment/?projectId=${pid}&assessmentId=${assessmentId}`;
 await page.goto(reviewURL);
 await page.getByLabel('线索 1 处理决定').selectOption('create');
 await page.getByLabel('复核理由').fill('浏览器协议夹具：确认待核实线索');
 await page.getByRole('button',{name:'提交整批复核并继续',exact:true}).click();
 await page.getByText('当前研判不在可复核状态。',{exact:true}).waitFor();
 assert.equal(submitted.length,1);assert.equal(submitted[0].expectedRevision,1);assert.equal(submitted[0].decisions[0].action,'create');assert.deepEqual(submitted[0].decisions[0].evidenceRefs,['fixture-evidence']);
 reviewState='needs_review';reviewRevision=3;
 await page.reload();await page.getByLabel('线索 1 处理决定').selectOption('reject');
 await page.getByLabel('复核理由').fill('浏览器协议夹具：驳回线索');
 await page.screenshot({path:resolve(output,'assessment-review-fixture.png'),fullPage:true});
 await page.getByRole('button',{name:'提交整批复核并继续',exact:true}).click();
 await page.getByText('当前研判不在可复核状态。',{exact:true}).waitFor();
 assert.equal(submitted.length,2);assert.equal(submitted[1].decisions[0].action,'reject');
 reviewConflict=true;reviewState='needs_review';reviewRevision=5;await page.reload();
 await page.getByLabel('线索 1 处理决定').selectOption('reject');
 await page.getByLabel('复核理由').fill('并发复核冲突测试');
 await page.getByRole('button',{name:'提交整批复核并继续',exact:true}).click();
 await page.getByRole('alert').filter({hasText:'INSPECTION_REVIEW_REVISION_CONFLICT'}).waitFor();
 assert.equal(await page.getByLabel('复核理由').inputValue(),'并发复核冲突测试');
 assert.equal(submitted.length,2);
 reviewConflict=false;
 canReview=false;reviewState='needs_review';await page.reload();
 await page.getByText('当前账号可查看，不能提交复核。',{exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'提交整批复核并继续',exact:true}).count(),0);
 reviewSource='unknown';await page.reload();
 await page.getByText('识别来源：未标明识别来源',{exact:false}).waitFor();
 assert.equal(await page.getByText('识别来源：司空原生告警',{exact:false}).count(),0);
 const reportId='33333333-2222-4333-8444-555555555555';
 let reportContent={taskRun:{id:1},issues:[],inspection:{scopeNotice:'仅分析冻结样本',observations:[],assessments:[]}};
 let reportGaps=['observation:partial'];
 await page.route(`**/api/projects/${pid}/reports/${reportId}`,route=>route.fulfill({json:{id:reportId,title:'报告 UI 协议样本',version:2,status:'draft',completeness:'incomplete',content:reportContent,dataGaps:reportGaps,evidence:[{kind:'asset',id:'1',availability:'unavailable-or-version-changed'}]}}));
 await page.goto(`${origin}/projects/reports/detail/?projectId=${pid}&reportId=${reportId}`);
 await page.getByText('仅分析冻结样本',{exact:true}).waitFor();
 await page.getByRole('alert').filter({hasText:'部分原图已不可用'}).waitFor();
 await page.getByText('关联案件（0）',{exact:true}).waitFor();
 for(const [mode,label] of [['unknown','未标明来源'],['assets','既有图片'],['existing-flight','司空已完成飞行'],['flighthub-flight','司空飞行']]) {
  reportContent={taskRun:{id:1},issues:[],inspection:{scopeNotice:'仅分析冻结样本',observations:[{id:'source-label-fixture',mode,scopeDescription:'来源标签协议样本',completeness:'partial',assets:[],observedFrom:'2026-09-14T00:00:00Z',observedTo:'2026-09-14T00:00:00Z'}],assessments:[]}};
  await page.reload();
  await page.getByText(`${label} · 0 张图片 · partial`,{exact:true}).waitFor();
  if(mode!=='existing-flight'&&await page.getByText('司空已完成飞行 · 0 张图片 · partial',{exact:true}).count())throw new Error('report falsely claimed completed flight');
 }
 reportContent={sections:{taskRun:{id:1},issues:[{id:1,title:'历史报告案件',status:'open'}]}};
 reportGaps=[{message:'历史资料缺失',code:'SOURCE_MISSING'}];
 await page.reload();
 await page.getByRole('link',{name:'历史报告案件',exact:true}).waitFor();
 await page.getByText('历史资料缺失',{exact:true}).waitFor();
 await page.screenshot({path:resolve(output,'report-legacy-fixture.png'),fullPage:true});
 reportContent={evidence:[]};reportGaps=[];
 await page.reload();
 await page.getByText('关联案件（0）',{exact:true}).waitFor();
 assert.deepEqual(errors,[]);
 writeFileSync(resolve(output,'result.json'),JSON.stringify({passed:true,checks:['YAML load','form updates source','custom nested data retained','JSON round trip','invalid source retained','real API save and reload','schedule form changes','inspection template draft via UI','alert policy enable and reload','explicit legacy release','alert policy disable','formal API and background runtime report without device','successful Run trigger replay','formal assets observe and report runtime','sealed PNG checksum and original provenance','assessment UI fixture confirm and reject','assessment UI read-only permission','report runtime and legacy shapes','report explicit source labels and unknown provenance','report missing sections','report stale evidence warning','authenticated original image preview','changed bytes preview refused','real runtime zero-case report page','review revision conflict preserves input','real Run summary navigation and refresh','pending review summary fixture and assessment link','empty-project readiness and refresh','formal browser review creates real case and report','formal browser rejection creates no case','formal report to case navigation','formal two-page stale review conflict preserves input','manual run lost response retries same committed Run','enabled Task displays its execution delegate','flight plan scoped selection and YAML version preview without action'],physicalActions:false,restarts},null,2));
 console.log(`Inspection authoring browser acceptance passed: ${output}`);
}finally{
 await browser?.close();
 if(app&&app.exitCode===null&&app.signalCode===null){const exited=once(app,'exit');app.kill('SIGTERM');await Promise.race([exited,delay(10000)]);if(app.exitCode===null&&app.signalCode===null){app.kill('SIGKILL');await exited;}}
 closeSync(log);
 if(created){assert(name.startsWith('aerosight_test_author_'));await admin.query(`DROP DATABASE "${name}" WITH (FORCE)`);}
 await admin.end();
}
