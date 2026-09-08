import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';

export async function callbackRecoveryFixture({ docker, database, app, project, team, request, upstream, objectRoot }) {
  assert(Number.isSafeInteger(project.id) && Number.isSafeInteger(team.id));
  const sql = statement => docker('exec', database, 'psql', '-U', 'postgres', '-v', 'ON_ERROR_STOP=1', '-Atqc', statement);
  const assetBytes = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII=', 'base64');
  const checksum = createHash('sha256').update(assetBytes).digest('hex');
  const assetKey = `projects/${project.id}/callback.png`;
  docker('exec', app, 'sh', '-c', 'mkdir -p "$1" && printf "%s" "$2" | base64 -d > "$1/callback.png"', 'sh', `${objectRoot}/projects/${project.id}`, assetBytes.toString('base64'));
  const providerId = Number(sql(`
    INSERT INTO algorithm_providers(project_id,team_id,name,provider_type,base_url,status)
    VALUES(${project.id},${team.id},'Restart callback','http-json','${upstream.endpoint}','active') RETURNING id`));
  const assetId = Number(sql(`
    INSERT INTO assets(project_id,team_id,kind,storage_key,logical_key,status,mime_type,checksum_sha256)
    VALUES(${project.id},${team.id},'image','${assetKey}','${assetKey}','available','image/png','${checksum}') RETURNING id`));
  assert(Number.isSafeInteger(providerId) && providerId > 0);
  assert(Number.isSafeInteger(assetId) && assetId > 0);
  const definition = await (await request(`/api/projects/${project.id}/algorithm-definitions`, 201, {
    definition: { providerId, name: 'Restart callback', capabilityCode: 'detection' },
    configuration: { executionMode: 'callback', modelOrProcess: 'test', inputSchema: {}, parametersSchema: {}, outputSchema: {},
      protocolConfig: {}, outputMapping: { detectionsPath: 'results' } },
  })).json();
  const { runId: run } = await (await request(`/api/projects/${project.id}/algorithm-runs`, 202, {
    configurationSnapshotId: definition.configurationSnapshotId, assetId, parameters: {},
  })).json();
  assert.match(run, /^[0-9a-f-]{36}$/);
  let input;
  for (let n = 0; n < 100; n++) {
    const received = upstream.read();
    if (received.length) { assert.equal(received.length, 1); input = received[0]; break; }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert(input, 'outbox did not dispatch the API-created run to the HTTPS upstream');
  assert.equal(input.runId, run);
  assert.equal(input.inputAsset.assetId, assetId);
  assert.equal(input.callback.url, `https://aerosight.test/callbacks/algorithms/${run}`);
  const token = input.callback.token;
  assert(token.length >= 32);
  for (let n = 0; ; n++) {
    if (sql(`SELECT status FROM algorithm_runs WHERE id='${run}'`) === 'waiting_callback') break;
    assert(n < 100, 'upstream acceptance was not committed');
    await new Promise(resolve => setTimeout(resolve, 100));
  }
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
      const signed = new URL(input.inputAsset.accessUrl);
      assert.equal(signed.origin, 'https://aerosight.test');
      const asset = await fetch(origin + signed.pathname + signed.search, { signal: AbortSignal.timeout(5000) });
      assert.equal(asset.status, 200);
      assert.deepEqual(Buffer.from(await asset.arrayBuffer()), assetBytes);
      assert.equal(state().receipts, 0);
      // The real upstream's 202 already moved the run to waiting_callback.
      // A processing receipt is retained, while its redundant state change is a no-op.
      assert.equal((await callback(origin, 'processing-before-stop', 'processing')).duplicate, true);
      assert.equal(state().receipts, 1);
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
      assert.equal(upstream.read().length, 1, 'restart dispatched the accepted run again');
      assert.equal(sql(`SELECT count(*) FROM algorithm_run_attempts WHERE algorithm_run_id='${run}' AND status='succeeded'`), '1');
      return { runId: run, createdThroughAPI: true, httpsUpstreamRequests: 1, signedAssetBytesVerified: true, waitingStatePreserved: true, receiptReplayAfterRestart: true, completionAppliedOnce: true, result: completed,
        scope: 'Provider and input image are fixtures. Definition/run creation, outbox dispatch to a trusted HTTPS upstream, signed asset HTTP delivery, issued callback credentials, restart and replay are real. Object persistence is checked after the next restart.' };
    },
    async verifyPersistedResult(origin, completed) {
      assert.deepEqual(state(), completed);
      assert.equal(completed.objectKey, `projects/${project.id}/algorithm-runs/${run}/raw-result.json`);
      const resultPath = `${objectRoot}/${completed.objectKey}`;
      assert.equal(docker('exec', app, 'sha256sum', resultPath).split(/\s+/)[0], completed.checksum);
      assert.deepEqual(JSON.parse(docker('exec', app, 'cat', resultPath)), { results: [] });
      const signed = new URL(input.inputAsset.accessUrl);
      const response = await fetch(origin + signed.pathname + signed.search, { signal: AbortSignal.timeout(5000) });
      assert.equal(response.status, 200);
      assert.deepEqual(Buffer.from(await response.arrayBuffer()), assetBytes);
      assert.equal((await callback(origin, 'complete-after-restart', 'completed')).duplicate, true);
      assert.deepEqual(state(), completed);
      assert.equal(upstream.read().length, 1);
      return { storage: 'named-volume', rawResultChecksumVerified: true, rawResultJSONVerified: true,
        inputAssetHTTPBytesPreserved: true, completedCallbackReplayPreserved: true, result: completed };
    },
  };
}
