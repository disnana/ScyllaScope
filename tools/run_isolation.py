#!/usr/bin/env python3
"""Run bounded RSS isolation cases sequentially, one fresh process per case.
Build the memory.counters test binary first; never compiles during measurement.
"""
import argparse, datetime, hashlib, json, os, pathlib, signal, subprocess, time

PLAN=[('idle',300,120),('sqlite_fixed',900,180),('collector_fixed',900,180),('json_only',600,180),('json_sse',900,180),('notification',900,180)]
def utc():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def save(path,value):
    temp=path.with_suffix('.tmp');temp.write_text(json.dumps(value,indent=2)+'\n');temp.replace(path)
def main():
    p=argparse.ArgumentParser();p.add_argument('binary');p.add_argument('output');p.add_argument('--source-commit',required=True);p.add_argument('--compact',action='store_true');p.add_argument('--smoke',action='store_true');p.add_argument('--modes',help='Explicit comma-separated subset, preserving listed order');p.add_argument('--compact-seconds',type=int,default=1200);p.add_argument('--seconds',type=int,help='Duration override for exactly one explicit mode');a=p.parse_args()
    if len(a.source_commit)!=40 or any(c not in '0123456789abcdef' for c in a.source_commit):p.error('source commit must be full lowercase SHA')
    binary=pathlib.Path(a.binary).resolve();root=pathlib.Path(a.output).resolve();root.mkdir(mode=0o700,parents=True,exist_ok=False)
    if not 300<=a.compact_seconds<=1200:p.error('compact duration must be 300..1200 seconds')
    plan=PLAN+([('sqlite_compact',a.compact_seconds,180)] if a.compact else [])
    if a.modes:
        requested=a.modes.split(',');known={x[0]:x for x in plan}
        if len(set(requested))!=len(requested) or any(x not in known for x in requested):p.error('invalid or duplicate mode subset')
        plan=[known[x] for x in requested]
    if a.seconds is not None:
        if not a.modes or len(plan)!=1 or not plan[0][2]<a.seconds<=1800:p.error('seconds override requires one explicit mode and duration above its warmup, at most 1800')
        plan=[(plan[0][0],a.seconds,plan[0][2])]
    if a.smoke:plan=[(mode,10,2) for mode,_,_ in plan]
    assert sum(x[1] for x in plan)<=5700
    manifest={'source_commit':a.source_commit,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'started_utc':utc(),'plan':plan,'sequential':True,'fresh_process_per_case':True,'GOMEMLIMIT':'128MiB','GOGC':'100','GOMAXPROCS':'2','measured_seconds_limit':5700,'cases':[],'complete':False}
    save(root/'manifest.json',manifest)
    for mode,seconds,warmup in plan:
        case=root/mode;case.mkdir(mode=0o700)
        env=os.environ.copy();env.update({'SCYLLASCOPE_ISOLATION_ENABLE':'1','SCYLLASCOPE_ISOLATION_MODE':mode,'SCYLLASCOPE_ISOLATION_SECONDS':str(seconds),'SCYLLASCOPE_ISOLATION_WARMUP':str(warmup),'SCYLLASCOPE_ISOLATION_SAMPLE_SECONDS':'5','SCYLLASCOPE_ISOLATION_OUTPUT':str(case),'GOMEMLIMIT':'128MiB','GOGC':'100','GOMAXPROCS':'2'})
        entry={'mode':mode,'started_utc':utc(),'seconds':seconds,'warmup_seconds':warmup,'state':'running'};manifest['cases'].append(entry);save(root/'manifest.json',manifest)
        with open(case/'test.log','xb') as log:
            proc=subprocess.Popen([str(binary),'-test.run=^TestIsolationMemory$','-test.count=1',f'-test.timeout={seconds+45}s','-test.v'],env=env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            started=time.monotonic();last_progress=started
            while proc.poll() is None:
                elapsed=time.monotonic()-started
                reason=None
                if elapsed>seconds+60:reason='wall_timeout'
                try:
                    rss=int(pathlib.Path(f'/proc/{proc.pid}/statm').read_text().split()[1])*os.sysconf('SC_PAGE_SIZE')
                    if rss>256*(1<<20):reason='rss_limit'
                except (FileNotFoundError,IndexError,ValueError):pass
                rows=case/'samples.jsonl'
                if elapsed>40 and (not rows.exists() or time.time()-rows.stat().st_mtime>30):reason='sampler_stalled'
                if reason:
                    try:os.killpg(proc.pid,signal.SIGKILL)
                    except ProcessLookupError:pass
                    proc.wait();entry['state']=reason;break
                if time.monotonic()-last_progress>=30:
                    rows=case/'samples.jsonl';row={}
                    if rows.exists():
                        try:row=json.loads(rows.read_text().splitlines()[-1])
                        except (ValueError,IndexError):pass
                    print(json.dumps({'mode':mode,'elapsed_seconds':round(time.monotonic()-started),'rss_bytes':row.get('rss_bytes'),'native_mapped_bytes':row.get('allocator_mapped_bytes'),'operations':row.get('operations')}),flush=True);last_progress=time.monotonic()
                time.sleep(1)
            entry.update({'finished_utc':utc(),'wall_seconds':round(time.monotonic()-started,3),'exit_code':proc.returncode})
            if entry['state']=='running':entry['state']='pass' if proc.returncode==0 else 'failed'
        save(root/'manifest.json',manifest)
        print(json.dumps({'mode':mode,'state':entry['state'],'finished_utc':entry['finished_utc']}),flush=True)
        if entry['state']!='pass':raise SystemExit(f'{mode} stopped; inspect private case log; remaining cases were not run')
    manifest.update({'complete':True,'finished_utc':utc()});save(root/'manifest.json',manifest)
if __name__=='__main__':main()
