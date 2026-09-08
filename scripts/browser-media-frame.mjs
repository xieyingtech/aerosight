import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';

export async function verifyMediaFrame(page, context, detailURL, output, seedSQL) {
  const mediaDirectory = resolve(output, 'hls-fixture');
  mkdirSync(mediaDirectory, { recursive: true });
  const generated = spawnSync('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=24', '-t', '6', '-an', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-g', '24', '-sc_threshold', '0', '-f', 'hls', '-hls_time', '1', '-hls_playlist_type', 'vod', '-hls_segment_filename', resolve(mediaDirectory, 'segment%d.ts'), resolve(mediaDirectory, 'index.m3u8')], { encoding: 'utf8', timeout: 30000 });
  assert.equal(generated.status, 0, `FFmpeg fixture generation failed: ${generated.stderr || generated.error}`);
  const mediaFiles = new Set(readdirSync(mediaDirectory));
  const hlsRequests = [];
  const hlsPattern = 'https://media.example/hls/**';
  await page.route(hlsPattern, route => {
    const name = new URL(route.request().url()).pathname.split('/').at(-1);
    if (!mediaFiles.has(name)) return route.fulfill({ status: 404 });
    hlsRequests.push(name);
    return route.fulfill({ headers: { 'Access-Control-Allow-Origin': '*' }, contentType: name.endsWith('.m3u8') ? 'application/vnd.apple.mpegurl' : 'video/mp2t', body: readFileSync(resolve(mediaDirectory, name)) });
  });
  const base = new URL(detailURL);
  const projectId = base.searchParams.get('projectId');
  assert(/^[1-9][0-9]*$/.test(projectId));
  const seed = JSON.parse(seedSQL(`WITH scope AS (SELECT id,team_id FROM projects WHERE id=${projectId}),
    profile AS (INSERT INTO device_network_profiles(project_id,team_id,name,mode,media_playback_base_url,config_json)
      SELECT id,team_id,'CSP media','lan','https://media.example/hls/','{"webrtcPlaybackBaseUrl":"https://media.example/rtc/"}' FROM scope RETURNING id),
    adapter AS (UPDATE device_adapters SET network_profile_id=profile.id FROM profile WHERE project_id=${projectId} AND name='CSP map adapter' RETURNING device_adapters.id,project_id,team_id),
    stream AS (INSERT INTO live_streams(project_id,team_id,device_id,adapter_id,stream_key,source_type,status,playback_ref,last_active_at)
      SELECT adapter.project_id,team_id,devices.id,adapter.id,'csp-media','dji','live','demo/aerosight/csp-media',now() FROM adapter JOIN devices ON devices.adapter_id=adapter.id RETURNING id,device_id)
    SELECT json_build_object('streamId',id,'deviceId',device_id) FROM stream;`));
  const mediaPattern = 'https://media.example/rtc/**';
  let frameURL;
  await page.route(mediaPattern, route => {
    frameURL = new URL(route.request().url());
    return route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body><p>Media origin accepted</p></body></html>' });
  });
  await page.route('https://demotiles.maplibre.org/style.json', route => route.fulfill({ json: { version: 8, sources: {}, layers: [] } }));
  try {
    await page.goto(`${base.origin}/projects/realtime/?projectId=${projectId}&deviceId=${seed.deviceId}&streamId=${seed.streamId}`);
    await page.frameLocator('iframe[title="WebRTC 直播"]').getByText('Media origin accepted', { exact: true }).waitFor({ state: 'visible' });
    assert(frameURL?.searchParams.get('token'), 'Go playback URL lacks a signed token');
    const auth = await context.request.post(`${base.origin}/api/media-auth`, { data: { action: 'read', protocol: 'webrtc', path: 'demo/aerosight/csp-media', token: frameURL.searchParams.get('token') } });
    assert.equal(auth.status(), 204, 'signed playback token rejected');
    assert.deepEqual(await page.evaluate(() => window.cspViolations), [], 'allowed media frame blocked');
    await page.getByRole('button', { name: '切换备用协议', exact: true }).click();
    await page.waitForFunction(() => {
      const video = document.querySelector('video');
      return video && video.videoWidth > 0 && video.currentTime > 0.5 && !video.error;
    });
    const playback = await page.locator('video').evaluate(video => ({ width: video.videoWidth, height: video.videoHeight, currentTime: video.currentTime }));
    assert(hlsRequests.includes('index.m3u8') && hlsRequests.some(name => name.endsWith('.ts')), 'HLS manifest or segments not fetched');
    assert.deepEqual(await page.evaluate(() => window.cspViolations), [], 'HLS CSP violations');
    await page.screenshot({ path: resolve(output, 'hls-playback.png'), fullPage: true });
    await page.evaluate(() => { const frame = document.createElement('iframe'); frame.src = 'https://unapproved-media.example/'; document.body.append(frame); });
    await page.waitForFunction(() => window.cspViolations.some(item => item.directive === 'frame-src' && item.blocked === 'https://unapproved-media.example'));
    return { streamId: seed.streamId, allowedOrigin: frameURL.origin, signedTokenAccepted: true, unapprovedOriginBlocked: true, hlsRequests, playback, scope: 'frame CSP, Go playback authorization and actual HLS video decode using generated VOD segments; fixture HTML does not establish a WebRTC media session' };
  } finally {
    await page.unroute(mediaPattern);
    await page.unroute(hlsPattern);
    await page.unroute('https://demotiles.maplibre.org/style.json');
    await page.goto(detailURL);
  }
}
