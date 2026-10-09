# SQL dependency inventory: a starting point for manual runtime usage review.
# Excludes generated code and tests from production evidence and links sqlc
# definitions to actual callers. INSERT/UPSERT can consume state through
# constraints and RETURNING without a separate SELECT; zero reads is not proof
# that a table is unused. Runtime registration and event producers require review.
from pathlib import Path
import re,json
root=Path(__file__).resolve().parents[1]; internal=root/'apps/server/internal'
schema=(root/'db/schema.sql').read_text(encoding='utf8')
tables=sorted(set(re.findall(r'^CREATE TABLE public\.([a-z0-9_]+)\s*\(',schema,re.M)))
views={m[0]:set(re.findall(r'\b(?:FROM|JOIN)\s+(?:public\.)?(\w+)',m[1],re.I)) for m in re.findall(r'^CREATE VIEW public\.(\w+)[\s\S]*?AS\s+([\s\S]*?);',schema,re.M)}
go_files=[p for p in (root/'apps/server').rglob('*.go') if 'sqlcgen' not in p.parts and 'migrations' not in p.parts]
runtime=[p for p in go_files if not p.name.endswith('_test.go') and '.build' not in p.parts and 'cmd' not in p.parts]
sources={p:p.read_text(encoding='utf8') for p in runtime}
results={t:{'reads':[],'writes':[],'unused_queries':[],'tests':[],'fk_in':[],'fk_out':[]} for t in tables}
def ref(sql):
 found=[]
 for m in re.finditer(r'\b(insert\s+into|update|delete\s+from|from|join)\s+(?:only\s+)?(?:public\.)?([a-z_]\w*)',sql,re.I):
  op='writes' if m[1].lower().split()[0] in ('insert','update','delete') else 'reads';name=m[2].lower()
  targets={name}|views.get(name,set())
  for t in targets:
   if t in results:found.append((t,op,name))
 return set(found)
def site(p,text,at):return str(p.relative_to(root)).replace('\\','/')+':'+str(text.count('\n',0,at)+1)
for p,text in sources.items():
 # Scan SQL literals only; words in identifiers, comments and struct fields aren't dependencies.
 for m in re.finditer(r'`([^`]+)`|"((?:[^"\\]|\\.)*)"',text):
  sql=m[1] or m[2]
  for t,op,name in ref(sql):results[t][op].append({'site':site(p,text,m.start()),'via':name})
for p in (internal/'database/queries').glob('*.sql'):
 text=p.read_text(encoding='utf8')
 headers=list(re.finditer(r'^-- name: (\w+) :\w+',text,re.M))
 for index,h in enumerate(headers):
  name=h[1];sql=text[h.end():headers[index+1].start() if index+1<len(headers) else len(text)]
  calls=[site(p2,s,m.start()) for p2,s in sources.items() for m in re.finditer(r'\.\s*'+re.escape(name)+r'\s*\(',s)]
  for t,op,via in ref(sql):
   record={'site':site(p,text,h.start()),'query':name,'calls':calls,'via':via}
   results[t][op if calls else 'unused_queries'].append(record)
for p in go_files:
 if not p.name.endswith('_test.go'):continue
 text=p.read_text(encoding='utf8')
 for t in tables:
  if re.search(r'\b'+t+r'\b',text):results[t]['tests'].append(str(p.relative_to(root)).replace('\\','/'))
for m in re.finditer(r'ALTER TABLE ONLY public\.(\w+)\s+ADD CONSTRAINT (\w+) FOREIGN KEY \(([^)]+)\) REFERENCES public\.(\w+)\(([^)]+)\)([^;]*);',schema):
 t,name,columns,parent,parentcols,rest=m.groups()
 if t in results:results[t]['fk_out'].append({'parent':parent,'columns':columns,'constraint':name})
 if parent in results:results[parent]['fk_in'].append({'child':t,'columns':columns,'constraint':name})
(root/'.build').mkdir(exist_ok=True)
(root/'.build/table-audit.json').write_text(json.dumps({'tables':results,'views':{k:sorted(v) for k,v in views.items()}},indent=2),encoding='utf8')
for t,data in results.items():
 if not data['writes'] or not data['reads']:
  print(t, 'R='+str(len(data['reads'])),'W='+str(len(data['writes'])),'Qunused='+str(len(data['unused_queries'])),'FKin='+str(len(data['fk_in'])))
print('Audited',len(results),'tables;',len(views),'views. Detailed SQL sites and caller links in .build/table-audit.json')
