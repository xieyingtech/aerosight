import assert from 'node:assert/strict';

export async function verifyAIFlow({ request, upstream, project }) {
  await request('/api/admin/ai-providers', 201, { name: 'Lifecycle AI', providerType: 'openai',
    baseUrl: 'https://algorithm.test:8443/v1', modelId: 'lifecycle-model', apiKey: 'lifecycle-ai-key', enabled: true, isDefault: true });
  const base = `/api/projects/${project.id}/agent-sessions`;
  const session = await (await request(base, 201, {})).json();
  assert(Number.isSafeInteger(session.id));
  const path = `${base}/${session.id}/messages`;
  const answer = await (await request(path, 201, { content: '查询当前项目' })).json();
  assert.equal(answer.content, '项目查询已完成。');
  assert.equal(answer.modelId, 'openai:lifecycle-model');
  const calls = upstream.readAI();
  assert.equal(calls.length, 2);
  assert(calls.every(call => call.authenticated));
  assert.equal(calls[0].body.model, 'lifecycle-model');
  assert.equal(calls[0].body.store, false);
  assert.equal(calls[0].body.tools.length, 6);
  const outputs = calls[1].body.input.filter(item => item.type === 'function_call_output');
  assert.equal(outputs.length, 6);
  for (const item of outputs) {
    const result = JSON.parse(item.output);
    assert.equal(result.projectId, project.id);
    assert.equal(result.quality, 'authoritative-project-query');
  }
  const history = async () => {
    const rows = await (await request(base, 200)).json();
    return rows.find(row => row.id === session.id).messages;
  };
  const messages = await history();
  assert.equal(messages.length, 2);
  assert.equal(messages[1].role, 'assistant');
  assert.equal(messages[1].toolCalls.length, 6);
  const refs = messages[1].toolCalls.flatMap(call => call.evidenceRefs ?? []);
  assert(refs.some(ref => ref.type === 'map-context' && ref.href.includes(`projectId=${project.id}`)));
  const failure = await (await request(path, 400, { content: 'acceptance failure' })).json();
  assert.equal(failure.error, 'AI_UPSTREAM_FAILED');
  assert(!JSON.stringify(failure).includes('private-upstream-detail'));
  const retained = await history();
  assert.equal(retained.length, 3);
  assert.equal(retained.filter(row => row.role === 'assistant').length, 1);
  assert.equal(upstream.readAI().length, 3, 'failed call was retried unexpectedly');
  let pending, waitingSession, waitingHistory;
  return {
    async beforeStop() {
      waitingSession = await (await request(base, 201, {})).json();
      assert(Number.isSafeInteger(waitingSession.id));
      pending = request(`${base}/${waitingSession.id}/messages`, 400, { content: 'acceptance wait' }, { timeoutMs: 60000 })
        .then(response => response.json()).then(value => ({ value }), error => ({ error }));
      const deadline = Date.now() + 5000;
      while (upstream.readAIHold().started !== 1) {
        assert(Date.now() < deadline, 'AI request did not reach the held upstream');
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      assert.deepEqual(upstream.readAIHold(), { started: 1, closed: 0 });
      const rows = await (await request(base, 200)).json();
      waitingHistory = rows.find(row => row.id === waitingSession.id).messages;
      assert.equal(waitingHistory.length, 1);
      assert.equal(waitingHistory[0].role, 'user');
    },
    async afterStop() {
      let timer;
      try {
        const result = await Promise.race([pending, new Promise((_, reject) => {
          timer = setTimeout(() => reject(new Error('AI client did not finish after Go shutdown')), 2000);
        })]);
        if (result.error) throw result.error;
        assert.equal(result.value.error, 'REQUEST_CANCELLED');
      } finally { clearTimeout(timer); }
      assert.deepEqual(upstream.readAIHold(), { started: 1, closed: 1 }, 'Go did not close the upstream request');
    },
    async afterRestart() {
      assert.deepEqual(await history(), retained);
      const rows = await (await request(base, 200)).json();
      assert.deepEqual(rows.find(row => row.id === waitingSession.id).messages, waitingHistory);
      assert.equal(upstream.readAI().length, 4, 'restart replayed AI requests');
      assert.deepEqual(upstream.readAIHold(), { started: 1, closed: 1 });
      return { sessionId: session.id, responsesRequests: 4, readToolsExecuted: 6, authenticatedHTTPS: true,
        scopedOutputs: true, evidenceRetained: true, upstreamFailureSanitized: true, historyPreservedAfterRestart: true,
        activeRequestCancelledOnSIGTERM: true, upstreamConnectionClosed: true, cancelledTurnHasNoAssistant: true };
    },
  };
}
