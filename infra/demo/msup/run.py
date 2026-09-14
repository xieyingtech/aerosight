import json,time
from datetime import datetime,timezone
from pathlib import Path
import httpx
out=Path('.build/msup-demo'); state=json.loads((out/'demo-state.json').read_text())
c=httpx.Client(base_url='http://127.0.0.1:8092',timeout=90,headers={'Origin':'http://127.0.0.1:8092'})
def post(path,body):
 token=c.get('/api/auth/csrf').json()['csrfToken']; r=c.post(path,json=body,headers={'X-CSRF-Token':token});
 if r.status_code>=400:raise RuntimeError(r.text)
 return r.json()
post('/api/auth/login',{'username':'admin@example.com','password':'admin'})
prefix=f"/api/projects/{state['project']['id']}"
if 'run' not in state:
 state['run']=post(prefix+f"/tasks/{state['task']['taskId']}/runs",{'type':'manual','idempotencyKey':'msup-real-demo-3','occurredAt':datetime.now(timezone.utc).isoformat().replace('+00:00','Z'),'inputs':{}})
 (out/'demo-state.json').write_text(json.dumps(state,ensure_ascii=False,indent=2))
print('Run',state['run'],flush=True)
last=None
for _ in range(100):
 r=c.get(prefix+f"/task-runs/{state['run']['taskRunId']}");r.raise_for_status(); model=r.json()
 status=(model['run']['status'],[(s.get('key'),s['status']) for s in model['steps']])
 if status!=last: print(status,flush=True);last=status
 (out/'run-detail.json').write_text(json.dumps(model,ensure_ascii=False,indent=2))
 if model['run']['status'] in ['failed','paused','succeeded','canceled']:break
 time.sleep(2)
