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
    const requests = [], aiRequests = [];
    const aiHold = {started:0,closed:0};
    https.createServer({key:fs.readFileSync('/fixture/key.pem'),cert:fs.readFileSync('/fixture/cert.pem')}, async (req,res) => {
      res.setHeader('Content-Type','application/json');
      if(req.method==='GET' && req.url==='/received') return res.end(JSON.stringify(requests));
      if(req.method==='GET' && req.url==='/ai-received') return res.end(JSON.stringify(aiRequests));
      if(req.method==='GET' && req.url==='/ai-hold') return res.end(JSON.stringify(aiHold));
      if(req.method==='POST' && req.url==='/v1/responses') {
        let raw=''; for await(const chunk of req) raw+=chunk;
        const body=JSON.parse(raw);
        aiRequests.push({body,authenticated:req.headers.authorization==='Bearer lifecycle-ai-key'});
        if(body.input.some(item=>item.role==='user' && item.content==='acceptance wait')) {
          aiHold.started++;res.on('close',()=>aiHold.closed++);return;
        }
        if(body.input.some(item=>item.role==='user' && item.content==='acceptance failure')) {
          res.writeHead(503);return res.end(JSON.stringify({error:{message:'private-upstream-detail'}}));
        }
        const continuation=body.input.some(item=>item.type==='function_call_output');
        const output=continuation ? [{type:'message',id:'msg_fixture',role:'assistant',status:'completed',content:[{type:'output_text',text:'项目查询已完成。',annotations:[]}]}]
          : ['query_devices','query_tasks','query_issues','query_assets','query_tracks','query_map_context'].map((name,i)=>({type:'function_call',id:'fc_'+i,call_id:'call_'+i,name,arguments:'{}',status:'completed'}));
        return res.end(JSON.stringify({id:'resp_fixture',object:'response',status:'completed',output}));
      }
      if(req.method!=='POST' || req.url!=='/run') { res.writeHead(404); return res.end('{}'); }
      let body=''; for await(const chunk of req) { body+=chunk; if(body.length>1048576) { res.writeHead(413);return res.end('{}'); } }
      requests.push(JSON.parse(body)); res.writeHead(202);res.end(JSON.stringify({externalJobId:'restart-job'}));
    }).listen(8443,'0.0.0.0');
  `;
  docker('run', '-d', '--name', name, '--network', network, '--network-alias', 'algorithm.test', '--read-only',
    '--mount', `type=bind,src=${key},dst=/fixture/key.pem,readonly`, '--mount', `type=bind,src=${cert},dst=/fixture/cert.pem,readonly`,
    'node:22-bookworm-slim', 'node', '-e', program);
  const readPath = path => JSON.parse(docker('exec', '-e', 'NODE_EXTRA_CA_CERTS=/fixture/cert.pem', name, 'node', '-e',
    `require('node:https').get('https://algorithm.test:8443${path}',r=>{let s='';r.on('data',c=>s+=c);r.on('end',()=>process.stdout.write(s));}).on('error',()=>process.exit(1));`));
  const read = () => readPath('/received');
  try {
    for (let n = 0; ; n++) {
      try { read(); break; } catch (error) { if (n === 30) throw error; }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  } catch (error) { docker('rm', '-f', name); throw error; }
  return { read, readAI: () => readPath('/ai-received'), readAIHold: () => readPath('/ai-hold'), endpoint: 'https://algorithm.test:8443/run',
    installTrust: app => docker('exec', app, 'sh', '-c', 'printf "%s" "$1" > /tmp/algorithm-ca.pem', 'sh', readFileSync(cert, 'utf8')),
    close: () => docker('rm', '-f', name) };
}
