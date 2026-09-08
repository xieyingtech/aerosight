import assert from 'node:assert/strict';
import { createHash, createHmac, randomBytes, randomUUID } from 'node:crypto';

export function callbackRecoveryFixture({ docker, database, project, team }) {
  assert(Number.isSafeInteger(project.id) && Number.isSafeInteger(team.id));
  const token = randomBytes(32).toString('hex');
  const tokenHash = createHash('sha256').update(token).digest('hex');
  const run = randomUUID();
  const sql = statement => docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atqc', statement);
  const providerId = Number(sql(`WITH provider AS (
    INSERT INTO algorithm_providers(project_id,team_id,name,provider_type,base_url,status)
    VALUES(${project.id},${team.id},'Restart callback','http-json','https://algorithm.example','active') RETURNING id
  ), definition AS (
    INSERT INTO algorithm_definitions(project_id,team_id,provider_id,name,capability_code)
    SELECT ${project.id},${team.id},provider.id,'Restart callback','detection' FROM provider RETURNING id
  ), version AS (
    INSERT INTO algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json)
    SELECT ${project.id},${team.id},definition.id,1,'published','callback','test','{"detectionsPath":"results"}' FROM definition RETURNING id
  ), asset AS (
    INSERT INTO assets(project_id,team_id,kind,storage_key,logical_key,status,mime_type)
    VALUES(${project.id},${team.id},'image','callback.jpg','callback.jpg','available','image/jpeg') RETURNING id
  ), run AS (
    INSERT INTO algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status,external_job_id,callback_token_hash)
    SELECT '${run}',${project.id},${team.id},version.id,asset.id,'restart-callback','running','restart-job','${tokenHash}' FROM version,asset RETURNING id
  ) SELECT provider.id FROM provider,run`));
  assert(Number.isSafeInteger(providerId) && providerId > 0);
  const state = () => JSON.parse(sql(`SELECT json_build_object(
    'status',status,'objectKey',raw_result_object_key,'checksum',raw_result_checksum_sha256,
    'finishedAt',finished_at,'receipts',(SELECT count(*) FROM algorithm_callback_receipts WHERE algorithm_run_id='${run}')
    ) FROM algorithm_runs WHERE id='${run}'`));
  async function callback(origin, callbackID, status) {
    const timestamp = Math.floor(Date.now() / 1000);
    const body = JSON.stringify({ providerId, externalJobId: 'restart-job', status, ...(status === 'completed' ? { result: { results: [] } } : {}) });
    const signature = createHmac('sha256', token).update(`${timestamp}.${callbackID}.${body}`).digest('hex');
    const response = await fetch(`${origin}/callbacks/algorithms/${run}`, {
      method: 'POST', signal: AbortSignal.timeout(5000), body,
      headers: { 'Content-Type': 'application/json', 'X-Aerosight-Provider-Id': String(providerId),
        'X-Aerosight-Callback-Id': callbackID, 'X-Aerosight-Timestamp': String(timestamp),
        'X-Aerosight-Callback-Token': token, 'X-Aerosight-Signature': `sha256=${signature}` },
    });
    const payload = await response.json();
    assert.equal(response.status, 200, JSON.stringify(payload));
    return payload;
  }
  return {
    async beforeStop(origin) {
      assert.equal((await callback(origin, 'processing-before-stop', 'processing')).duplicate, false);
      assert.equal(state().status, 'waiting_callback');
    },
    async afterRestart(origin) {
      assert.deepEqual(state(), { status: 'waiting_callback', objectKey: null, checksum: null, finishedAt: null, receipts: 1 });
      assert.equal((await callback(origin, 'processing-before-stop', 'processing')).duplicate, true);
      assert.equal((await callback(origin, 'complete-after-restart', 'completed')).duplicate, false);
      const completed = state();
      assert.equal(completed.status, 'succeeded');
      assert.equal(completed.receipts, 2);
      assert(completed.objectKey && /^[a-f0-9]{64}$/.test(completed.checksum) && completed.finishedAt);
      assert.equal((await callback(origin, 'complete-after-restart', 'completed')).duplicate, true);
      assert.deepEqual(state(), completed);
      return { runId: run, waitingStatePreserved: true, receiptReplayAfterRestart: true, completionAppliedOnce: true, result: completed,
        scope: 'Seeded in-flight run; real signed HTTP callbacks before/after restart and receipt replay. Upstream execution and object survival across another restart are separate checks.' };
    },
  };
}
