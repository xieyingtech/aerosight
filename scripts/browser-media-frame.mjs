import assert from 'node:assert/strict';

export async function verifyMediaFrame(page, context, detailURL, seedSQL) {
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
    await page.locator('iframe[title="WebRTC 直播"]').evaluate(frame => { frame.src = 'https://unapproved-media.example/'; });
    await page.waitForFunction(() => window.cspViolations.some(item => item.directive === 'frame-src' && item.blocked === 'https://unapproved-media.example'));
    return { streamId: seed.streamId, allowedOrigin: frameURL.origin, signedTokenAccepted: true, unapprovedOriginBlocked: true, scope: 'frame CSP and Go playback authorization; fixture HTML does not establish a WebRTC media session' };
  } finally {
    await page.unroute(mediaPattern);
    await page.unroute('https://demotiles.maplibre.org/style.json');
    await page.goto(detailURL);
  }
}
