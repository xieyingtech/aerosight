import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {resolve} from 'node:path';

// Only the completed observation/detection/model stage is a seeded fixture.
// Human review, continuation, case writes and report generation use the real API
// and background runtime. No provider or aircraft is called by this fixture.
export async function verifyRealInspectionReview({page,db,post,pid,teamId,origin,output,restartServer}) {
 const number=async(sql,args=[])=>Number((await db.query(sql,args)).rows[0].id);
 const uid=await number("select id from users where email='admin@example.com'");
 const agent=await number("select id from agents where project_id=$1 and config_json->>'kind'='copilot'",[pid]);
 const count=async()=>Number((await db.query('select count(*) as id from issues where project_id=$1',[pid])).rows[0].id);
 const initialCount=await count();
 for(const action of ['create','reject']) {
  const source={apiVersion:'aerosight/v2',name:`浏览器正式复核 ${action}`,trigger:{type:'manual'},steps:[
   {key:'observe',uses:'inspection.observe',with:{mode:'assets',assetIds:[1]}},
   {key:'detect',uses:'inspection.detect',with:{observationId:'steps.observe.outputs.observationId',source:'external',algorithmDefinitionVersionId:1}},
   {key:'assess',uses:'copilot.run',with:{mode:'assessment',evidenceSetId:'steps.detect.outputs.evidenceSetId'}},
   {key:'issue',uses:'issue.create-or-update',with:{assessmentId:'steps.assess.outputs.assessmentId'}},
   {key:'report',uses:'report.generate'}]};
  const created=await post(`/api/projects/${pid}/tasks`,{sourceFormat:'json',source:JSON.stringify(source),idempotencyKey:`browser-real-review-${action}`});
  assert.equal(created.status,201,JSON.stringify(created));
  const task=created.body.taskId,version=created.body.versionId;
  const observationId=randomUUID(),evidenceId=randomUUID(),assessmentId=randomUUID(),algorithmId=randomUUID();
  // Seed a paused, published snapshot so this acceptance starts exactly at the
  // review boundary. Publication/provider acceptance is tested separately.
  await db.query('begin');
  let run;
  try {
   await db.query("update task_versions set status='published' where id=$1",[version]);
   await db.query("update tasks set status='active' where id=$1",[task]);
   run=await number("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id,input_snapshot_json) values($1,$2,$3,$4,'manual','paused',$5,'{}') returning id",[pid,teamId,task,version,uid]);
   const steps={};
   for(const step of (await db.query('select id,position,step_key from task_steps where task_version_id=$1 order by position',[version])).rows) {
    const status=step.position<3?'succeeded':step.position===3?'paused':'pending';
    const snapshot=step.step_key==='observe'?{observationId}:step.step_key==='detect'?{evidenceSetId:evidenceId}:step.step_key==='assess'?{assessmentId}:{};
    steps[step.step_key]=await number('insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status,output_snapshot_json) values($1,$2,$3,$4,$5,$6,$7) returning id',[pid,teamId,run,step.id,step.position,status,JSON.stringify(snapshot)]);
   }
   const asset=await number("insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image',$3,$3,'available') returning id",[pid,teamId,`browser-review-${action}`]);
   const scope={projectId:pid,teamId};
   const assetRef={...scope,assetId:asset,version:1,checksumSha256:'a'.repeat(64)};
   const now=new Date().toISOString();
   const observation={id:observationId,contractVersion:'aerosight/inspection/v1',run:{...scope,runId:run,stepId:steps.observe},mode:'assets',assets:[assetRef],completeness:'complete',scopeDescription:'协议样本：浏览器人工复核联动，不含实拍或模型验收',observedFrom:now,observedTo:now};
   const ref=`algorithm:${algorithmId}`,candidateId=`external:${algorithmId}:0`;
   const evidence={id:evidenceId,run:{...scope,runId:run,stepId:steps.detect},observationId,source:'external',modelVersion:'seeded-protocol-fixture',completeness:'complete',targetAlgorithmConfirmed:true,evidenceRefs:[ref],candidates:[{id:candidateId,evidenceRefs:[ref],position:{source:'unknown',quality:'image-only'}}],externalResults:[{ref,algorithmRunId:algorithmId,asset:assetRef,result:{kind:'detection',detections:[{detectionKey:`browser-${action}`,label:'candidate',confidence:0.9}]}}]};
   const decisions=[{candidateId,action:'needs_review',reason:'需要人工核实协议样本',evidenceRefs:[ref],missingInformation:['人工核实']}];
   await db.query("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','complete',$6,now(),now(),$7,now())",[observationId,pid,teamId,run,steps.observe,observation.scopeDescription,JSON.stringify(observation)]);
   await db.query("insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version,completeness,target_algorithm_confirmed,evidence_json) values($1,$2,$3,$4,$5,$6,'external','seeded-protocol-fixture','complete',true,$7)",[evidenceId,pid,teamId,run,steps.detect,observationId,JSON.stringify(evidence)]);
   await db.query("insert into inspection_assessments(id,project_id,team_id,task_run_id,task_run_step_id,evidence_set_id,status,revision,original_output,model_version,prompt_version) values($1,$2,$3,$4,$5,$6,'needs_review',1,$7,'seeded-protocol-fixture','fixture-v1')",[assessmentId,pid,teamId,run,steps.assess,evidenceId,JSON.stringify({decisions})]);
   await db.query("insert into inspection_assessment_revisions(assessment_id,project_id,revision,source,decisions_json,idempotency_key) values($1,$2,1,'model',$3,'seeded-model')",[assessmentId,pid,JSON.stringify(decisions)]);
   const session=await number('insert into agent_sessions(project_id,agent_id,task_run_id,started_by_user_id) values($1,$2,$3,$4) returning id',[pid,agent,run,uid]);
   await db.query("insert into agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,tool_name,required_permission,args_json,status,context_expires_at) values($1,$2,$3,$4,'inspection_assessment','agent:use',$5,'succeeded',now()+interval '1 day')",[pid,teamId,session,uid,JSON.stringify({assessmentId})]);
   await db.query('commit');
  } catch(error) {await db.query('rollback');throw error;}
  const before=await count();
  if(action==='create') {
   await restartServer('paused-review-before-human-decision');
   const persisted=await db.query('select status from task_runs where id=$1',[run]);
   assert.equal(persisted.rows[0].status,'paused');
   assert.equal(await count(),before);
  }
  let stalePage;
  if(action==='create') {
   stalePage=await page.context().newPage();
   await stalePage.goto(`${origin}/projects/inspection/assessment/?projectId=${pid}&assessmentId=${assessmentId}`);
   await stalePage.getByLabel('线索 1 处理决定').selectOption('reject');
   await stalePage.getByLabel('复核理由').fill('并发旧修订：必须保留输入并拒绝');
  }

  if(action==='create') {
   await page.goto(`${origin}/projects/tasks/runs/detail/?projectId=${pid}&runId=${run}`);
   const detectStep=page.locator('details').filter({has:page.getByText('inspection.detect',{exact:true})});
   await detectStep.locator('summary').click();
   const opened=page.waitForEvent('popup');
   await detectStep.getByRole('link',{name:'识别证据',exact:true}).click();
   const evidencePage=await opened;
   await evidencePage.getByRole('heading',{name:'识别证据',exact:true}).waitFor();
   await evidencePage.getByText('模型版本：seeded-protocol-fixture',{exact:false}).waitFor();
   await evidencePage.getByText('位置：未知',{exact:true}).waitFor();
   const evidenceRoute=`**/api/projects/${pid}/inspection/evidence-sets/${evidenceId}`;
   await evidencePage.route(evidenceRoute,route=>route.abort('internetdisconnected'));
   await evidencePage.getByRole('button',{name:'刷新识别证据',exact:true}).click();
   await evidencePage.getByText('加载失败，请重试。',{exact:true}).waitFor();
   assert.equal(await evidencePage.getByText('位置：未知',{exact:true}).count(),0);
   await evidencePage.unroute(evidenceRoute);
   await evidencePage.getByRole('button',{name:'重试',exact:true}).click();
   await evidencePage.getByText('位置：未知',{exact:true}).waitFor();
   await evidencePage.getByRole('link',{name:'查看观察范围与原图',exact:true}).click();
   await evidencePage.getByText('来源：既有图片',{exact:false}).waitFor();
   await evidencePage.getByText('协议样本：浏览器人工复核联动，不含实拍或模型验收',{exact:true}).waitFor();
   await evidencePage.close();
   const assessStep=page.locator('details').filter({has:page.getByText('copilot.run',{exact:true})});
   if(await assessStep.getAttribute('open')===null)await assessStep.locator('summary').click();
   await assessStep.getByRole('link',{name:'查看研判与人工复核',exact:true}).click();
   await page.waitForURL(url=>url.searchParams.get('assessmentId')===assessmentId);
  } else {
   await page.goto(`${origin}/projects/inspection/assessment/?projectId=${pid}&assessmentId=${assessmentId}`);
  }
  await page.getByText('位置需核实',{exact:false}).waitFor();
  await page.getByLabel('线索 1 处理决定').selectOption(action);
  await page.getByLabel('复核理由').fill(`浏览器正式接口 ${action}，仅验证协议样本`);
  const response=page.waitForResponse(r=>r.url().endsWith(`/assessments/${assessmentId}/review`)&&r.request().method()==='POST');
  await page.getByRole('button',{name:'提交整批复核并继续',exact:true}).click();
  const review=await response;assert.equal(review.status(),200,await review.text());
  const reviewBody=review.request().postDataJSON();
  if(stalePage) {
   const conflictResponse=stalePage.waitForResponse(r=>r.url().endsWith(`/assessments/${assessmentId}/review`)&&r.request().method()==='POST');
   await stalePage.getByRole('button',{name:'提交整批复核并继续',exact:true}).click();
   const conflict=await conflictResponse;assert.equal(conflict.status(),409,await conflict.text());
   await stalePage.getByRole('alert').filter({hasText:'INSPECTION_REVIEW_REVISION_CONFLICT'}).waitFor();
   assert.equal(await stalePage.getByLabel('复核理由').inputValue(),'并发旧修订：必须保留输入并拒绝');
   await stalePage.close();
  }

  let model;
  for(let i=0;i<160;i++) {
   model=await page.evaluate(async path=>{const r=await fetch(path);if(!r.ok)throw new Error(await r.text());return r.json();},`/api/projects/${pid}/task-runs/${run}`);
   if(['succeeded','failed'].includes(model.run.status))break;
   await new Promise(resolve=>setTimeout(resolve,250));
  }
  assert.equal(model.run.status,'succeeded',JSON.stringify(model));
  assert.equal(await count(),before+(action==='create'?1:0));
  const issueStep=model.steps.find(step=>step.key==='issue');
  assert.equal(issueStep.status,'succeeded');
  assert.equal(issueStep.outputSnapshot.issueIds.length,action==='create'?1:0);
  const original=await db.query('select original_output,revision from inspection_assessments where id=$1',[assessmentId]);
  assert.equal(original.rows[0].revision,2);assert.match(original.rows[0].original_output,/needs_review/);
  const reportId=model.steps.find(step=>step.key==='report').outputSnapshot.reportId;
  if(action==='create') {
   const snapshot=await db.query('select id,status,output_snapshot_json from task_run_steps where task_run_id=$1 order by id',[run]);
   await restartServer('completed-case-and-report-before-review-replay');
   const replay=await post(`/api/projects/${pid}/inspection/assessments/${assessmentId}/review`,reviewBody);
   assert.equal(replay.status,200,JSON.stringify(replay));
   const restored=await db.query('select id,status,output_snapshot_json from task_run_steps where task_run_id=$1 order by id',[run]);
   assert.deepEqual(restored.rows,snapshot.rows);
   assert.equal(await count(),before+1);
   const revisions=await db.query('select revision from inspection_assessments where id=$1',[assessmentId]);
   assert.equal(revisions.rows[0].revision,2);
  }
  await page.goto(`${origin}/projects/reports/detail/?projectId=${pid}&reportId=${reportId}`);
  await page.getByText(`关联案件（${action==='create'?1:0}）`,{exact:true}).waitFor();
  await page.screenshot({path:resolve(output,`real-review-${action}-report.png`),fullPage:true});
  if(action==='create') {
   await page.getByRole('link',{name:'巡检待核实线索',exact:true}).click();
   await page.waitForURL(url=>url.pathname.includes('/issues/detail/')&&url.searchParams.get('issueId')===String(issueStep.outputSnapshot.issueIds[0]));
   await page.getByRole('heading',{name:/案件 #.*巡检待核实线索/}).waitFor();
   await page.getByText('协作处置',{exact:true}).waitFor();
  }
 }
 assert.equal(await count(),initialCount+1);
}
