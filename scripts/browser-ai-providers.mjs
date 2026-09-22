import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { chromium } from 'playwright';

// UI contract test against the running web development server. All API calls are
// intercepted, so this never edits the operator's providers or credentials.
const origin = process.env.AI_PROVIDER_BROWSER_ORIGIN ?? 'http://localhost:3000';
const output = resolve('.build/ai-provider-browser');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1360, height: 1000 } });
  const failures = [];
  page.on('pageerror', error => failures.push(error.message));
  let providers = [];
  let discoveryFails = false;
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let data;
    let status = 200;
    if (path === '/api/auth/session') data = { user: { id: 1, name: '管理员', email: 'test@example.test', role: 'admin' } };
    else if (path === '/api/auth/csrf') data = { csrfToken: 'fixture-token' };
    else if (path === '/api/projects') data = [];
    else if (path === '/api/admin/ai-providers/models') {
      data = discoveryFails ? { error: 'AI_PROVIDER_MODELS_FAILED' } : { models: ['local-text', 'local-voice'] };
      if (discoveryFails) status = 400;
    } else if (path === '/api/admin/ai-providers/defaults') {
      const {kind,providerId,modelId} = request.postDataJSON();
      providers = providers.map(p => kind === 'text' ? {...p,isDefault:p.id===providerId,modelId:p.id===providerId?modelId:p.modelId} : {...p,isRealtimeDefault:p.id===providerId,realtimeModelId:p.id===providerId?modelId:p.realtimeModelId});
      data = {ok:true};
    } else if (path.endsWith('/test')) data = { ok: true, code: 'OK' };
    else if (path === '/api/admin/ai-providers' && request.method() === 'GET') data = providers;
    else if (path === '/api/admin/ai-providers' && request.method() === 'POST') {
      const body = request.postDataJSON();
      assert.equal(body.baseUrl, 'http://192.168.1.10:8000/v1');
      const provider = { ...body, id: String(providers.length + 1), status: 'untested', health: {}, lastTestedAt: null, updatedAt: new Date().toISOString() };
      providers.push(provider); data = provider; status = 201;
    } else if (request.method() === 'PATCH') {
      const id = path.split('/').at(-1);
      providers = providers.map(p => p.id === id ? { ...p, ...request.postDataJSON() } : p);
      data = providers.find(p => p.id === id);
    } else if (request.method() === 'DELETE') {
      providers = providers.filter(p => p.id !== path.split('/').at(-1)); data = { deleted: true };
    } else throw new Error(`Unexpected API: ${request.method()} ${path}`);
    await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) });
  });
  await page.goto(`${origin}/admin/ai-providers`);
  await page.getByRole('button', { name: '新建 Provider', exact: true }).waitFor();
  assert.equal(await page.getByRole('textbox').count(), 0, 'no inline create form');
  await page.getByRole('button', { name: '新建 Provider', exact: true }).click();
  await page.getByLabel('供应商名称', { exact: true }).fill('内网模型服务');
  await page.getByLabel('基础地址', { exact: true }).fill('http://192.168.1.10:8000/v1');
  await page.getByRole('tab', { name: /模型配置/ }).click();
  await page.getByRole('status').filter({ hasText: '2 个可用模型' }).waitFor();
  const addModel = page.getByRole('combobox', { name: '添加模型', exact: true });
  await addModel.fill('text');
  await page.getByRole('option', { name: 'local-text', exact: true }).click();
  await addModel.fill('voice');
  await page.getByRole('option', { name: 'local-voice', exact: true }).click();
  await page.getByLabel('local-voice 调用协议', { exact: true }).selectOption('stepfun-realtime');
  await addModel.fill('custom-model');
  await addModel.press('Enter');
  await page.screenshot({ path: resolve(output, 'model-dialog.png'), fullPage: true });
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal(providers[0].models.length, 3);
  assert.equal(providers[0].models[0].protocol, 'openai-compatible');
  await page.getByLabel('默认文字模型', {exact:true}).selectOption(JSON.stringify(['1','local-text']));
  await page.getByLabel('默认实时模型', {exact:true}).selectOption(JSON.stringify(['1','local-voice']));
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  assert.equal(await page.getByLabel('供应商名称', { exact: true }).inputValue(), '内网模型服务');
  assert.equal(await page.getByLabel('API Key', { exact: true }).inputValue(), '');
  await page.getByRole('tab', { name: /模型配置/ }).click();
  discoveryFails = true;
  await page.getByRole('button', { name: '刷新模型列表', exact: true }).click();
  await page.getByRole('status').filter({ hasText: '模型列表加载失败' }).waitFor();
  assert.equal(await page.getByLabel('模型 1 ID', { exact: true }).inputValue(), 'local-text');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal(providers[0].isDefault, true);
  assert.equal(providers[0].isRealtimeDefault, true);
  await page.screenshot({ path: resolve(output, 'provider-list.png'), fullPage: true });
  await page.getByRole('button', { name: '测试', exact: true }).click();
  await page.getByRole('status').filter({ hasText: 'API 连接正常' }).waitFor();
  await page.setViewportSize({ width: 485, height: 791 });
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('tab', { name: /模型配置/ }).click();
  for (let i = 0; i < 15; i++) {
    await addModel.fill(`scroll-test-${i}`);
    await addModel.press('Enter');
  }
  const scroll = page.getByTestId('model-matrix-scroll');
  const matrix = await scroll.boundingBox();
  const save = await page.getByRole('button', { name: '保存配置', exact: true }).boundingBox();
  assert(matrix.y + matrix.height < save.y, 'matrix cannot overlap save footer');
  await scroll.hover();
  await page.mouse.wheel(500, 1000);
  assert(await scroll.evaluate(el => el.scrollTop > 0), 'model list scrolls vertically');
  await page.getByRole('dialog').waitFor();
  const bounds = await page.getByRole('dialog').boundingBox();
  assert(bounds.x >= 0 && bounds.x + bounds.width <= 486, 'mobile dialog fits viewport');
  await page.screenshot({ path: resolve(output, 'mobile-dialog.png'), fullPage: true });
  await page.getByRole('button', { name: '取消', exact: true }).click();
  assert.deepEqual(failures, []);
  console.log('Provider UI passed: modal creation/edit, automatic discovery, searchable/custom selection, protocol/global default selection, failed discovery retention, and scroll/overlap regression at 485x791.');
} finally { await browser.close(); }
