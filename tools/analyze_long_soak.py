#!/usr/bin/env python3
"""Summarize synthetic soak metrics without embedding private profiles/logs.
OLS slopes describe this finite workload, not a long-term leak guarantee.
"""
import argparse, json, statistics
from pathlib import Path

def slope(rows,key):
    if len(rows)<2:return None
    xs=[r['elapsed_seconds']/3600 for r in rows];ys=[r[key] for r in rows]
    xm=statistics.mean(xs);ym=statistics.mean(ys);den=sum((x-xm)**2 for x in xs)
    return sum((x-xm)*(y-ym) for x,y in zip(xs,ys))/den if den else None

def analyze(rows,warmup):
    post=[r for r in rows if r['phase']=='sample' and r['elapsed_seconds']>=warmup]
    blocks={}
    for r in post:blocks.setdefault(int((r['elapsed_seconds']-warmup)//300),[]).append(r)
    medians=[]
    for index,group in sorted(blocks.items()):
        if len(group)<5:continue
        medians.append({key:statistics.median(r[key] for r in group) for key in ['elapsed_seconds','heap_alloc_bytes','heap_objects','rss_bytes']})
    tail=[r for r in post if r['elapsed_seconds']>=max(warmup,rows[-1]['elapsed_seconds']-1200)]
    result={'warmup_seconds':warmup,'samples_after_warmup':len(post),'first':rows[0],'last':rows[-1],'series':rows,'five_minute_medians':medians,'trends':{},'bounds':{}}
    for key in ['heap_alloc_bytes','heap_objects','rss_bytes']:
        result['trends'][key]={'post_warmup_ols_per_hour':slope(post,key),'last_20_minutes_ols_per_hour':slope(tail,key),'five_minute_median_ols_per_hour':slope(medians,key),'post_warmup_min':min((r[key] for r in post),default=None),'post_warmup_max':max((r[key] for r in post),default=None)}
    for key in ['known','fds','goroutines','durable_work']:
        result['bounds'][key]={'min':min(r[key] for r in rows),'max':max(r[key] for r in rows)}
    for key in ['tracks','inflight','attempts','queued']:
        result['bounds']['alerts_'+key]={'min':min(r['alerts'][key] for r in rows),'max':max(r['alerts'][key] for r in rows)}
    result['limitations']=['Synthetic discovery and loopback probes, not long-term real CQL allocation','Explicit GC samples still contain concurrent request working sets','Slopes are descriptive, with correlated samples and periodic workload','A finite run cannot establish resolution of a weeks-long production issue']
    return result

def main():
    p=argparse.ArgumentParser();p.add_argument('samples');p.add_argument('--warmup',type=int,default=300);p.add_argument('--output');a=p.parse_args()
    rows=[json.loads(x) for x in Path(a.samples).read_text().splitlines() if x]
    result=analyze(rows,a.warmup);body=json.dumps(result,ensure_ascii=False,indent=2)+'\n'
    if a.output:Path(a.output).write_text(body)
    else:print(body,end='')
if __name__=='__main__':main()
