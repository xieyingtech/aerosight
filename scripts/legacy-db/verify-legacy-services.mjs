import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { openSync, closeSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { resolve } from 'node:path';
import { chromium } from 'playwright';

async function freePort() {
  const server = createServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}

export async function verifyLegacyServices({ release, output, databaseURL, scope, password, secret }) {
  const webPort = await freePort(), workerPort = await freePort();
  const origin = `http://127.0.0.1:${webPort}`;
  const env = { ...process.env, NODE_ENV: 'production', DATABASE_URL: databaseURL, AUTH_SECRET: secret,
    AUTH_URL: origin, AUTH_TRUST_HOST: 'true', CALLBACK_LISTEN_ADDRESS: `127.0.0.1:${workerPort}`,
    OBJECT_STORAGE_LOCAL_ROOT: resolve(output, 'objects'), DJI_FLIGHTHUB_ENABLED: 'false',
    CALLBACK_PUBLIC_BASE_URL: '', MEDIA_API_BASE_URL: '', MEDIA_ADMIN_USER: '', MEDIA_ADMIN_PASSWORD: '' };
  const children = [], logs = [];
  let browser;
  const start = (name, executable, args, cwd) => {
    const log = openSync(resolve(output, `${name}.log`), 'w'); logs.push(log);
    const child = spawn(executable, args, { cwd, env, stdio: ['ignore', log, log] });
    child.on('error', error => { child.startError = error; }); children.push(child);
    return child;
  };
  const ready = async (url, child) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      assert(!child.startError && child.exitCode === null, 'legacy process exited before readiness');
      try { const response = await fetch(url, { signal: AbortSignal.timeout(1000) }); await response.text(); if (response.ok) return; } catch {}
      await new Promise(resolve => setTimeout(resolve, 200));
    }
    throw new Error(`Legacy readiness timeout: ${url}`);
  };
  try {
    const web = start('legacy-web', process.execPath, [resolve(release, 'apps/web/node_modules/next/dist/bin/next'), 'start', '--hostname', '127.0.0.1', '--port', String(webPort)], resolve(release, 'apps/web'));
    const worker = start('legacy-worker', resolve(release, '.build', process.platform === 'win32' ? 'aerosight-worker.exe' : 'aerosight-worker'), [], release);
    await ready(`${origin}/login`, web);
    await ready(`http://127.0.0.1:${workerPort}/readyz`, worker);
    browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || (process.platform === 'win32' ? 'msedge' : undefined), headless: true });
    const page = await browser.newPage();
    await page.goto(`${origin}/login`);
    await page.getByLabel('邮箱或手机号').fill('rollback@example.test');
    await page.getByLabel('密码', { exact: true }).fill(password);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await page.waitForURL('**/projects');
    const session = await (await page.request.get(`${origin}/api/auth/session`)).json();
    assert.equal(session.user.userId, scope.userId, 'legacy login returned the wrong account');
    await page.goto(`${origin}/projects/${scope.projectId}`);
    await page.getByRole('heading', { name: 'rollback-project', exact: true }).waitFor();
    await page.reload();
    await page.getByRole('heading', { name: 'rollback-project', exact: true }).waitFor();
    const response = await page.request.get(`${origin}/api/projects/${scope.projectId}/snapshot`);
    assert.equal(response.status(), 200, 'legacy snapshot API failed against upgraded schema');
    const snapshot = await response.json();
    assert.equal(snapshot.project.id, scope.projectId, 'legacy snapshot returned the wrong project');
    assert(JSON.stringify(snapshot).includes('legacy-drone'), 'legacy snapshot did not return the preserved device');
    await page.screenshot({ path: resolve(output, 'legacy-project.png'), fullPage: true });
    assert.equal(worker.exitCode, null, 'legacy worker failed during browser access');
    return { login: true, accountId: session.user.userId, projectReload: true, snapshotAPI: true, workerReady: true,
      shutdownScope: 'test process cleanup; Windows forced cleanup is not graceful shutdown acceptance' };
  } finally {
    if (browser) await browser.close();
    for (const child of children.reverse()) {
      if (child.pid && child.exitCode === null && child.signalCode === null) {
        if (process.platform === 'win32') spawnSync('taskkill', ['/pid', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
        else child.kill('SIGKILL');
        if (child.exitCode === null && child.signalCode === null) await new Promise(resolve => child.once('exit', resolve));
      }
    }
    for (const log of logs) closeSync(log);
    writeFileSync(resolve(output, 'legacy-cleanup.json'), JSON.stringify({ processes: children.map(child => ({ exitCode: child.exitCode, signal: child.signalCode })) }, null, 2));
  }
}
