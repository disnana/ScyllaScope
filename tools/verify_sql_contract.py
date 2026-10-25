#!/usr/bin/env python3
"""Execute exact SQL from store.go with Python sqlite3, NOT the Go adapter."""
import json,math,re,sqlite3
from pathlib import Path
source=(Path(__file__).resolve().parents[1]/'internal/store/store.go').read_text()
raw=re.findall(r'`([^`]+)`',source)
conn=sqlite3.connect(':memory:')
schema=next(q for q in raw if q.startswith('CREATE TABLE IF NOT EXISTS events'))
conn.executescript(schema)
now=1_800_000_000_000;since=now-6*3_600_000
rows=[(since+i*1000,json.dumps({'ts':since+i*1000})) for i in range(21601)]
rows += [(now+i*3_600_000,json.dumps({'ts':now+i*3_600_000})) for i in range(1,5001)]
conn.executemany('INSERT INTO samples_v5 VALUES(?,?)',rows)
conn.executemany('INSERT INTO node_samples_v5 VALUES(?,?,?)',[('node-a',ts,b) for ts,b in rows])
conn.executemany('INSERT INTO node_samples_v5 VALUES(?,?,?)',[('node-b',ts,json.dumps({'foreign':True})) for ts,_ in rows])
results=[]
for limit in [2,3,17,500,2000]:
 step=max(1,math.ceil((now-since)/(limit-1)))
 for node in [False,True]:
  q=next(q for q in raw if q.startswith('SELECT body FROM '+('node_samples_v5' if node else 'samples_v5')))
  params=(('node-a','node-a') if node else ())+(since,now,since,step,limit)
  data=[json.loads(x[0]) for x in conn.execute(q,params)]
  assert 0<len(data)<=limit,(node,limit,len(data))
  assert all(since<=x['ts']<=now and 'foreign' not in x for x in data)
  assert data[-1]['ts']==now,(node,limit,data[-1])
  results.append({'test':'node-history' if node else 'history','limit':limit,'rows':len(data),'result':'PASS'})
plan=list(conn.execute('EXPLAIN QUERY PLAN DELETE FROM node_samples_v5 WHERE ts<?',(since,)))
assert any('node_samples_v5_ts_idx' in str(r) for r in plan),plan
results.append({'test':'node sample prune index','result':'PASS','plan':plan})
conn.executescript(schema)
assert conn.execute('SELECT count(*) FROM samples_v5').fetchone()[0]==len(rows)
results.append({'test':'additive migration preserves all existing rows','result':'PASS'})
print(json.dumps({'sqlite_version':sqlite3.sqlite_version,'scope':'exact SQL via Python sqlite3; Go modernc adapter NOT tested','results':results},indent=2))
