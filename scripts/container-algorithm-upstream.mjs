import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

export async function startAlgorithmUpstream({ docker, network, output }) {
  const name = `${network}-algorithm`;
  writeFileSync(resolve(output, 'openssl.cnf'), '[req]\ndistinguished_name=dn\n[dn]\n');
  const cert = resolve(output, 'algorithm-cert.pem');
  const key = resolve(output, 'algorithm-key.pem');
  const generated = spawnSync('openssl', ['req', '-config', resolve(output, 'openssl.cnf'), '-x509', '-newkey', 'rsa:2048', '-nodes',
    '-keyout', key, '-out', cert, '-days', '1', '-subj', '/CN=algorithm.test', '-addext', 'subjectAltName=DNS:algorithm.test'], { encoding: 'utf8' });
  assert.equal(generated.status, 0, generated.stderr);
  const program = `
    const https = require('node:https'), fs = require('node:fs');
    const requests = [];
    https.createServer({key:fs.readFileSync('/fixture/key.pem'),cert:fs.readFileSync('/fixture/cert.pem')}, async (req,res) => {
      res.setHeader('Content-Type','application/json');
      if(req.method==='GET' && req.url==='/received') return res.end(JSON.stringify(requests));
      if(req.method!=='POST' || req.url!=='/run') { res.writeHead(404); return res.end('{}'); }
      let body=''; for await(const chunk of req) { body+=chunk; if(body.length>1048576) { res.writeHead(413);return res.end('{}'); } }
      requests.push(JSON.parse(body)); res.writeHead(202);res.end(JSON.stringify({externalJobId:'restart-job'}));
    }).listen(8443,'0.0.0.0');
  `;
  docker('run', '-d', '--name', name, '--network', network, '--network-alias', 'algorithm.test', '--read-only',
    '--mount', `type=bind,src=${key},dst=/fixture/key.pem,readonly`, '--mount', `type=bind,src=${cert},dst=/fixture/cert.pem,readonly`,
    'node:22-bookworm-slim', 'node', '-e', program);
  const read = () => JSON.parse(docker('exec', '-e', 'NODE_EXTRA_CA_CERTS=/fixture/cert.pem', name, 'node', '-e',
    "require('node:https').get('https://algorithm.test:8443/received',r=>{let s='';r.on('data',c=>s+=c);r.on('end',()=>process.stdout.write(s));}).on('error',()=>process.exit(1));"));
  try {
    for (let n = 0; ; n++) {
      try { read(); break; } catch (error) { if (n === 30) throw error; }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  } catch (error) { docker('rm', '-f', name); throw error; }
  return { read, endpoint: 'https://algorithm.test:8443/run',
    installTrust: app => docker('exec', app, 'sh', '-c', 'printf "%s" "$1" > /tmp/algorithm-ca.pem', 'sh', readFileSync(cert, 'utf8')),
    close: () => docker('rm', '-f', name) };
}
