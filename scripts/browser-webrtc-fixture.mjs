import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { openSync, closeSync, writeFileSync, readFileSync } from 'node:fs';
import { createServer, request } from 'node:http';
import { createServer as createTLS } from 'node:https';
import { createServer as portServer } from 'node:net';
import { resolve } from 'node:path';

// TLS termination is a test fixture; signaling and ICE/TCP reach MediaMTX.
export async function startWebRTCFixture(output, apiPort, mediaPort) {
  const name = `aerosight-rtc-${randomUUID()}`;
  const image = 'bluenviron/mediamtx:1.20.1';
  const path = 'demo/aerosight/csp-media';
  const authPath = `/${randomUUID()}`;
  const authorizations = [];
  const auth = createServer(async (req, res) => {
    if (req.method !== 'POST' || req.url !== authPath) { res.writeHead(404); res.end(); return; }
    try {
      const chunks = []; let size = 0;
      for await (const chunk of req) { size += chunk.length; if (size > 16384) throw new Error('body limit'); chunks.push(chunk); }
      const body = Buffer.concat(chunks);
      const result = await fetch(`http://127.0.0.1:${apiPort}/api/media-auth`, { method: 'POST', headers: { 'content-type': 'application/json' }, body, signal: AbortSignal.timeout(5000) });
      const input = JSON.parse(body.toString());
      authorizations.push({ action: input.action, protocol: input.protocol, path: input.path, status: result.status });
      await result.arrayBuffer();
      res.writeHead(result.status); res.end();
    } catch { res.writeHead(502); res.end(); }
  });
  const command = (...args) => {
    const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000 });
    assert.equal(result.status, 0, `docker ${args[0]}: ${result.stderr || result.error}`);
    return result.stdout.trim();
  };
  let started = false, publisher, log, publisherError, tls;
  const close = async () => {
    writeFileSync(resolve(output, 'webrtc-authorizations.json'), JSON.stringify(authorizations, null, 2));
    if (tls) { tls.closeAllConnections(); await new Promise(resolve => tls.close(resolve)); }
    if (publisher && publisher.exitCode === null && publisher.signalCode === null) {
      const exited = new Promise(resolve => publisher.once('exit', resolve));
      publisher.kill(); await exited;
    }
    if (log !== undefined) closeSync(log);
    try { if (started) { writeFileSync(resolve(output, 'mediamtx.log'), command('logs', name)); command('stop', name); } }
    finally { auth.closeAllConnections(); await new Promise(resolve => auth.close(resolve)); }
  };
  try {
    await new Promise(resolve => auth.listen(0, '0.0.0.0', resolve));
    const ports = portServer();
    await new Promise(resolve => ports.listen(0, '127.0.0.1', resolve));
    const icePort = ports.address().port;
    await new Promise(resolve => ports.close(resolve));
    writeFileSync(resolve(output, 'mediamtx.yml'), `logLevel: info
authMethod: http
authHTTPAddress: http://host.docker.internal:${auth.address().port}${authPath}
authHTTPExclude:
  - action: publish
    path: ${path}
rtsp: true
rtspTransports: [tcp]
rtmp: false
hls: false
srt: false
moq: false
webrtc: true
webrtcLocalUDPAddress: ''
webrtcLocalTCPAddress: :${icePort}
webrtcIPsFromInterfaces: false
webrtcAdditionalHosts: [127.0.0.1]
paths:
  ${path}: {}
`);
    command('run', '--rm', '-d', '--name', name, '--add-host', 'host.docker.internal:host-gateway', '-p', '127.0.0.1::8554', '-p', '127.0.0.1::8889', '-p', `127.0.0.1:${icePort}:${icePort}`, '-v', `${output}:/fixture:ro`, image, '/fixture/mediamtx.yml');
    started = true;
    const rtspPort = command('port', name, '8554/tcp').split(':').at(-1);
    const httpPort = command('port', name, '8889/tcp').split(':').at(-1);
    const upstream = `http://127.0.0.1:${httpPort}`;
    tls = createTLS({ key: readFileSync(resolve(output, 'key.pem')), cert: readFileSync(resolve(output, 'cert.pem')) }, (req, res) => {
      if (!req.url.startsWith('/rtc/')) { res.writeHead(404); res.end(); return; }
      const target = request(`${upstream}${req.url.slice('/rtc'.length)}`, { method: req.method, headers: { ...req.headers, host: `127.0.0.1:${httpPort}` } }, reply => {
        const headers = { ...reply.headers };
        if (headers.location?.startsWith('/')) headers.location = `/rtc${headers.location}`;
        res.writeHead(reply.statusCode, headers); reply.pipe(res);
      });
      target.on('error', () => { if (!res.headersSent) res.writeHead(502); res.end(); });
      res.on('close', () => target.destroy()); req.pipe(target);
    });
    await new Promise(resolve => tls.listen(mediaPort, '127.0.0.1', resolve));
    for (let n = 0; n < 50; n++) {
      try { const response = await fetch(`${upstream}/${path}/`, { signal: AbortSignal.timeout(1000) }); await response.text(); if (response.ok || response.status === 401) break; } catch {}
      if (n === 49) throw new Error('MediaMTX startup timeout');
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    log = openSync(resolve(output, 'webrtc-publisher.log'), 'w');
    publisher = spawn('ffmpeg', ['-hide_banner', '-loglevel', 'warning', '-re', '-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=24', '-an', '-c:v', 'libx264', '-profile:v', 'baseline', '-pix_fmt', 'yuv420p', '-preset', 'ultrafast', '-tune', 'zerolatency', '-g', '24', '-f', 'rtsp', '-rtsp_transport', 'tcp', `rtsp://127.0.0.1:${rtspPort}/${path}`], { stdio: ['ignore', log, log] });
    publisher.on('error', error => { publisherError = error; });
    return { close, authorizations, assertPublisher: () => { assert(!publisherError, String(publisherError)); assert.equal(publisher.exitCode, null, 'FFmpeg publisher exited'); }, image };
  } catch (error) { await close(); throw error; }
}
