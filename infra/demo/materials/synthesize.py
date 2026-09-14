from pathlib import Path
import pathlib,json,urllib.request,os
ROOT=Path(__file__).resolve().parents[3]
os.chdir(ROOT)
env={}
for line in Path('.env.tts.local').read_text().splitlines():
 if '=' in line and not line.lstrip().startswith('#'):
  k,v=line.split('=',1);env[k.strip()]=v.strip().strip(chr(34)).strip(chr(39))
assert env.get('STEPFUN_API_KEY'), 'Missing STEPFUN_API_KEY'
assert env.get('STEPFUN_BASE_URL','').rstrip('/')=='https://api.stepfun.com/step_plan', 'Expected Step Plan base URL'

import base64,concurrent.futures,subprocess
out=pathlib.Path('.build/msup-demo/recording/timed');out.mkdir(parents=True,exist_ok=True)
segments=json.loads(pathlib.Path('docs/demo/narration.json').read_text())
def synth(item):
 i,s=item;target=out/f'{i:02d}.mp3';meta=out/f'{i:02d}.json'
 if target.exists() and meta.exists():return
 body={'model':'stepaudio-2.5-tts','voice':'cixingnansheng','input':s['text'],'response_format':'mp3','stream_format':'sse','timestamp':True,'instruction':'清晰自然的产品演示解说，语速适中，语气平稳。'}
 req=urllib.request.Request(env['STEPFUN_BASE_URL'].rstrip('/')+'/v1/audio/speech',data=json.dumps(body).encode(),headers={'Authorization':'Bearer '+env['STEPFUN_API_KEY'],'Content-Type':'application/json'})
 audio=bytearray();subs=[]
 with urllib.request.urlopen(req,timeout=240) as r:
  for line in r:
   if not line.startswith(b'data:'):continue
   data=line[5:].strip()
   if data==b'[DONE]':break
   e=json.loads(data)
   if e['type']=='speech.audio.delta':audio.extend(base64.b64decode(e['audio']))
   elif e['type']=='response.subtitle':subs.append(e['data'])
   elif e['type']=='speech.audio.error':raise RuntimeError(str(e))
 assert audio and subs,(i,'Missing audio/subtitles')
 target.write_bytes(audio);meta.write_text(json.dumps(subs,ensure_ascii=False,indent=2));print(i,len(audio),sum(len(x['items']) for x in subs),flush=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=2) as ex:list(ex.map(synth,enumerate(segments)))
