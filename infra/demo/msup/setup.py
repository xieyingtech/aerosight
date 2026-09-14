import json
import subprocess
from pathlib import Path
import httpx
root=Path.cwd(); out=root/'.build/msup-demo'; statefile=out/'demo-state.json'
state=json.loads(statefile.read_text()) if statefile.exists() else {}
def save():statefile.write_text(json.dumps(state,ensure_ascii=False,indent=2))
client=httpx.Client(base_url='http://127.0.0.1:8092',timeout=120,headers={'Origin':'http://127.0.0.1:8092'})
def call(path,body=None,method='POST',**kwargs):
 token=client.get('/api/auth/csrf').json()['csrfToken']
 r=client.request(method,path,json=body,headers={'X-CSRF-Token':token},**kwargs)
 if r.status_code>=400:raise RuntimeError(f'{path}: {r.status_code} {r.text[:1000]}')
 return r.json()
call('/api/auth/login',{'username':'admin@example.com','password':'admin'})
if 'team' not in state:state['team']=call('/api/teams',{'name':'MSUP 高校组演示'});save()
if 'project' not in state:state['project']=call('/api/projects',{'teamId':state['team']['id'],'name':'红花山历史航拍巡检 · 演示','description':'仅分析 2026-08-18 已下载历史图片；无飞行控制。'});save()
pid=state['project']['id']; prefix=f'/api/projects/{pid}'
if 'provider' not in state:
 state['provider']=call(prefix+'/algorithm-providers',{'name':'本地 YOLO11n','providerType':'http-json','baseUrl':'https://127.0.0.1:8444/infer','authType':'none','timeoutSeconds':60,'concurrencyLimit':2,'rateLimitPerMinute':60});save()
if 'algorithm' not in state:
 state['algorithm']=call(prefix+'/algorithm-definitions',body={'definition':{'providerId':state['provider']['id'],'name':'航拍人车目标检测 YOLO11n','capabilityCode':'detection'},'configuration':{'executionMode':'synchronous','modelOrProcess':'yolo11n-coco','inputSchema':{},'parametersSchema':{'type':'object'},'outputSchema':{},'protocolConfig':{},'outputMapping':{'kind':'detection','resultPath':'result.detections'}}});save()
if 'ai' not in state:
 config=json.loads(subprocess.check_output(['node','-e',"const fs=require('fs');console.log(JSON.stringify(require('node:util').parseEnv(fs.readFileSync('.env.inspection.local','utf8'))))"],text=True))
 state['ai']=call('/api/admin/ai-providers',{'name':'演示真实研判模型','providerType':'openai','baseUrl':config['AI_PROVIDER_BASE_URL'],'modelId':config['AI_PROVIDER_MODEL_ID'],'apiKey':config['AI_PROVIDER_API_KEY'],'enabled':True,'isDefault':True});save()
if 'asset' not in state:
 path=out/'source-media/rgb/images/DJI_20260818151715_0012_V.jpeg'
 with path.open('rb') as f:state['asset']=call(prefix+'/assets/import',files={'file':(path.name,f,'image/jpeg')},data={'capturedAt':'2026-08-18T15:17:15+08:00','sourceDescription':'用户从司空媒体目录 321463914 手动下载的历史航拍 RGB 原图；道路车辆巡查演示，无违规认定。'})
 save()
if 'task' not in state:
 source=f'''apiVersion: aerosight/v2
name: 红花山历史航拍巡查演示（正式闭环）
trigger:
  type: manual
concurrencyLimit: 1
steps:
  - key: observe
    uses: inspection.observe
    with:
      mode: assets
      assetIds: [{state['asset']['assetId']}]
      scopeDescription: 仅分析选定历史图片中的道路车辆，位置及违规性质需人工核对
  - key: detect
    uses: inspection.detect
    dependsOn: [observe]
    with:
      observationId: steps.observe.outputs.observationId
      source: external
      algorithmDefinitionVersionId: {state['algorithm']['configurationSnapshotId']}
      maxImages: 1
  - key: assess
    uses: copilot.run
    dependsOn: [detect]
    with:
      mode: assessment
      evidenceSetId: steps.detect.outputs.evidenceSetId
      temperature: 0.2
  - key: issue
    uses: issue.create-or-update
    dependsOn: [assess]
    with:
      assessmentId: steps.assess.outputs.assessmentId
  - key: report
    uses: report.generate
    dependsOn: [issue]
'''
 (out/'demo-task.yaml').write_text(source)
 state['task']=call(prefix+'/tasks',{'sourceFormat':'yaml','source':source,'idempotencyKey':'msup-demo-v2'});save()
if 'providerEnabled' not in state:
 body={k:state['provider'][k] for k in ['name','providerType','baseUrl','authType','allowedHeaders','timeoutSeconds','concurrencyLimit','rateLimitPerMinute']}
 body['status']='active'
 state['providerEnabled']=call(prefix+'/algorithm-providers/'+state['provider']['id'],body,method='PATCH');save()
if 'published' not in state:
 state['published']=call(prefix+f"/tasks/{state['task']['taskId']}/versions",{'action':'publish','versionId':state['task']['versionId'],'expectedRevision':1});save()
if 'active' not in state:
 state['active']=call(prefix+f"/tasks/{state['task']['taskId']}",{'status':'active'},method='PATCH');save()
print('Demo ready:', 'project', pid, 'task', state['task']['taskId'])
