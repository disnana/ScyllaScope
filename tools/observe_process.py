#!/usr/bin/env python3
"""Read-only Linux/systemd process observer, compatible with the old daemon.
No credentials, command lines, environment, database rows, or node addresses are read.
"""
from __future__ import annotations
import argparse,json,os,subprocess,sys,time
from pathlib import Path

def service_pid(service: str) -> int:
    result=subprocess.run(['systemctl','show','--property=MainPID','--value',service],capture_output=True,text=True,timeout=5,check=True)
    pid=int(result.stdout.strip())
    if pid<=0: raise RuntimeError('service has no running MainPID')
    return pid

def sample(pid: int) -> dict:
    root=Path('/proc')/str(pid)
    fields={line.partition(':')[0]:line.partition(':')[2].strip() for line in (root/'status').read_text().splitlines()}
    out={'time_utc':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'pid':pid}
    for key in ['VmRSS','VmHWM','RssAnon','RssFile','RssShmem','VmSwap']:
        parts=fields.get(key,'').split()
        if len(parts)==2 and parts[1]=='kB':out[key.lower()+'_bytes']=int(parts[0])*1024
    if 'Threads' in fields:out['threads']=int(fields['Threads'])
    try:out['open_fds']=sum(1 for _ in (root/'fd').iterdir())
    except PermissionError:out['open_fds_error']='permission denied (run as service owner or root)'
    # Optional cgroup v2 figures distinguish anonymous memory from file cache.
    for line in (root/'cgroup').read_text().splitlines():
        if line.startswith('0::'):
            base=Path('/sys/fs/cgroup').resolve();group=(base/line[3:].lstrip('/')).resolve()
            if not group.is_relative_to(base):break
            try:
                out['cgroup_memory_current_bytes']=int((group/'memory.current').read_text())
                stats=dict(row.split() for row in (group/'memory.stat').read_text().splitlines())
                out['cgroup_memory_anon_bytes']=int(stats.get('anon','0'))
                out['cgroup_memory_file_bytes']=int(stats.get('file','0'))
            except (OSError,ValueError):pass
            break
    return out

def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pid',type=int,help='otherwise resolve current systemd MainPID each sample')
    parser.add_argument('--service',default='scyllascope.service')
    parser.add_argument('--duration',type=float,default=900)
    parser.add_argument('--interval',type=float,default=5)
    parser.add_argument('--output',default='scyllascope-process.jsonl')
    args=parser.parse_args()
    if not 1<=args.duration<=604800 or not .1<=args.interval<=3600 or args.pid is not None and args.pid<1:
        parser.error('duration 1..604800s, interval .1..3600s, positive PID required')
    # Do not clobber a prior observation, follow a symlink, or create world-readable logs.
    flags=os.O_WRONLY|os.O_CREAT|os.O_EXCL|getattr(os,'O_NOFOLLOW',0)
    fd=os.open(args.output,flags,0o600);end=time.monotonic()+args.duration;errors=0
    with os.fdopen(fd,'w') as f:
        while True:
            try:row=sample(args.pid or service_pid(args.service))
            except (OSError,ValueError,RuntimeError,subprocess.SubprocessError) as e:
                errors+=1;row={'time_utc':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'error':str(e)}
            f.write(json.dumps(row)+'\n');f.flush()
            left=end-time.monotonic()
            if left<=0:break
            time.sleep(min(args.interval,left))
    print(f'Wrote {args.output}; failed samples={errors}',file=sys.stderr)
    return 1 if errors else 0
if __name__=='__main__':raise SystemExit(main())
