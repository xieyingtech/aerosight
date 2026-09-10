import assert from 'node:assert/strict';

export async function verifyMissionFlow({ docker, database, request, project, team, devices }) {
  assert([project.id, team.id, devices.gatewayID].every(Number.isSafeInteger));
  const sql = statement => docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atqc', statement);
  const run = Number(sql(`WITH task AS (
    INSERT INTO tasks(project_id,team_id,name,trigger_type,script)
    VALUES(${project.id},${team.id},'Lifecycle device task','manual','') RETURNING id
  ), version AS (
    INSERT INTO task_versions(project_id,team_id,task_id,version,script)
    SELECT ${project.id},${team.id},id,1,'' FROM task RETURNING id,task_id
  ), step AS (
    INSERT INTO task_steps(project_id,team_id,task_version_id,position,step_key,name,uses,action,capability_code,timeout_seconds,parameters_json)
    SELECT ${project.id},${team.id},id,1,'debug','Debug','device.command','debug.open','dock.debug.control',60,'{}' FROM version RETURNING id
  ), run AS (
    INSERT INTO task_runs(project_id,team_id,task_id,task_version_id,selected_device_id,trigger_source,status)
    SELECT ${project.id},${team.id},task_id,id,${devices.gatewayID},'manual','paused' FROM version RETURNING id
  ), run_step AS (
    INSERT INTO task_run_steps(project_id,team_id,task_run_id,task_step_id,position)
    SELECT ${project.id},${team.id},run.id,step.id,1 FROM run,step RETURNING task_run_id
  ) SELECT task_run_id FROM run_step`));
  assert(Number.isSafeInteger(run) && run > 0);
  const path = `/api/projects/${project.id}/task-runs/${run}`;
  const initial = await (await request(path, 200)).json();
  assert.equal(initial.run.status, 'paused');
  assert(initial.actions.includes('resume'));
  const beforeCount = devices.servicePublicationCount();
  const resumed = await (await request(path + '/control', 200, { action: 'resume', expectedVersion: initial.run.stateVersion, reason: 'Lifecycle acceptance' })).json();
  assert.equal(resumed.status, 'running');
  const deadline = Date.now() + 20000;
  while (sql(`SELECT status FROM task_runs WHERE id=${run}`) !== 'succeeded') {
    assert(Date.now() < deadline, 'MQTT command ACK did not complete the resumed task');
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  const workbench = await (await request(path, 200)).json();
  assert.equal(workbench.steps.length, 1);
  assert.equal(workbench.steps[0].status, 'succeeded');
  assert.equal(workbench.steps[0].commandStatus, 'acknowledged');
  const state = () => JSON.parse(sql(`SELECT json_build_object('status',r.status,'version',r.state_version,'finishedAt',r.finished_at,
    'commands',(SELECT count(*) FROM device_commands WHERE task_run_id=r.id),
    'attempts',(SELECT count(*) FROM command_attempts a JOIN device_commands c ON c.id=a.command_id WHERE c.task_run_id=r.id),
    'resumeAudits',(SELECT count(*) FROM audit_events WHERE project_id=${project.id} AND action='task_run.resume' AND resource_id='${run}'))
    FROM task_runs r WHERE r.id=${run}`));
  const completed = state();
  assert.equal(completed.commands, 1);
  assert.equal(completed.attempts, 1);
  assert.equal(completed.resumeAudits, 1);
  assert(completed.finishedAt);
  assert.equal(devices.servicePublicationCount() - beforeCount, 1);
  return {
    async afterRestart() {
      assert.deepEqual(state(), completed);
      const restored = await (await request(path, 200)).json();
      assert.equal(restored.run.status, 'succeeded');
      assert.equal(restored.steps[0].commandId, workbench.steps[0].commandId);
      assert.equal(devices.servicePublicationCount() - beforeCount, 1, 'restart republished the completed task command');
      return { runId: run, resumedThroughAPI: true, realMQTTAcknowledgement: true, servicePublications: 1,
        result: completed, preservedAfterRestart: true,
        scope: 'Paused task/version/step are fixtures because the migrated surface has no creation API. Resume, dispatch, MQTT service/ACK, completion and restart reads use the real application.' };
    },
  };
}
