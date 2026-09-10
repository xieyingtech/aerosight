import assert from 'node:assert/strict';

export async function verifyProjectDetails(page, detailURL, seedSQL) {
  const base = new URL(detailURL);
  const projectId = base.searchParams.get('projectId');
  assert(/^[1-9][0-9]*$/.test(projectId));
  const ids = JSON.parse(seedSQL(`WITH scope AS (SELECT id,team_id FROM projects WHERE id=${projectId}),
    task AS (INSERT INTO tasks(project_id,team_id,name,trigger_type,script) SELECT id,team_id,'Post-build task','manual','' FROM scope RETURNING id),
    task_run AS (INSERT INTO task_runs(project_id,team_id,task_id,trigger_source,status) SELECT scope.id,team_id,task.id,'manual','queued' FROM scope,task RETURNING id),
    issue AS (INSERT INTO issues(project_id,number,title,source_type) SELECT id,1,'Post-build issue','manual' FROM scope RETURNING id),
    provider AS (INSERT INTO algorithm_providers(project_id,team_id,name,provider_type,base_url,status) SELECT id,team_id,'Post-build provider','http-json','https://algorithm.example','active' FROM scope RETURNING id),
    definition AS (INSERT INTO algorithm_definitions(project_id,team_id,provider_id,name,capability_code) SELECT scope.id,team_id,provider.id,'Post-build algorithm','ocr' FROM scope,provider RETURNING id),
    version AS (INSERT INTO algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process) SELECT scope.id,team_id,definition.id,1,'published','synchronous','ocr-v1' FROM scope,definition RETURNING id),
    asset AS (INSERT INTO assets(project_id,team_id,kind,storage_key,logical_key,mime_type) SELECT id,team_id,'image','post-build.jpg','post-build.jpg','image/jpeg' FROM scope RETURNING id),
    algorithm_run AS (INSERT INTO algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status) SELECT gen_random_uuid(),scope.id,team_id,version.id,asset.id,'post-build-browser','succeeded' FROM scope,version,asset RETURNING id),
    rule AS (INSERT INTO event_rules(project_id,team_id,name) SELECT id,team_id,'Post-build rule' FROM scope RETURNING id),
    rule_version AS (INSERT INTO event_rule_versions(project_id,team_id,event_rule_id,version,label,minimum_confidence,severity) SELECT scope.id,team_id,rule.id,1,'construction',0.5,'medium' FROM scope,rule RETURNING id),
    detection_group AS (INSERT INTO detection_groups(project_id,team_id,label,location_quality,first_detected_at,last_detected_at) SELECT id,team_id,'construction','unavailable',now(),now() FROM scope RETURNING id),
    event AS (INSERT INTO perception_events(id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,severity,first_detected_at,last_detected_at) SELECT gen_random_uuid(),scope.id,team_id,rule_version.id,detection_group.id,'post-build-browser','medium',now(),now() FROM scope,rule_version,detection_group RETURNING id)
    SELECT json_build_object('taskRunId',(SELECT id FROM task_run),'algorithmRunId',(SELECT id FROM algorithm_run),'issueId',(SELECT id FROM issue),'eventId',(SELECT id FROM event),'teamId',scope.team_id,'teamName',teams.name) FROM scope JOIN teams ON teams.id=scope.team_id;`));
  const cases = [
    ['tasks/runs/detail', 'runId', ids.taskRunId, '任务运行工作台', 'Post-build task'],
    ['algorithms/runs/detail', 'runId', ids.algorithmRunId, 'Post-build algorithm'],
    ['issues/detail', 'issueId', ids.issueId, '案件 #1 · Post-build issue'],
    ['events/detail', 'eventId', ids.eventId, '疑似违建', 'Post-build rule'],
  ];
  const verified = [];
  for (const [path, key, id, heading, text] of cases) {
    const url = new URL(`/projects/${path}/`, base.origin);
    url.search = new URLSearchParams({ projectId, [key]: String(id) });
    await page.goto(url.href);
    for (const reload of [false, true]) {
      if (reload) await page.reload();
      await page.getByRole('heading', { name: heading, exact: true }).waitFor({ state: 'visible' });
      if (text) await page.getByText(text, { exact: true }).first().waitFor({ state: 'visible' });
      assert.equal(page.url(), url.href);
      assert.deepEqual(await page.evaluate(() => window.cspViolations), [], `${path} CSP`);
    }
    verified.push({ path, id, directVisit: true, reload: true });
  }
  await page.goto(`${base.origin}/teams/detail/?teamId=${ids.teamId}`);
  await page.getByRole('heading', { name: ids.teamName, exact: true }).waitFor({ state: 'visible' });
  await page.reload();
  await page.getByRole('heading', { name: ids.teamName, exact: true }).waitFor({ state: 'visible' });
  await page.getByRole('link', { name: 'Browser acceptance project', exact: true }).click();
  await page.getByRole('heading', { name: 'Browser acceptance project', exact: true }).waitFor({ state: 'visible' });
  assert.equal(new URL(page.url()).searchParams.get('projectId'), projectId);
  return { projectId, verified, teamDetail: true };
}
