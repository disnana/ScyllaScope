#!/usr/bin/env python3
"""Plot deidentified synthetic soak metrics (optional matplotlib dependency)."""
import argparse,json,os
os.environ.setdefault('MPLCONFIGDIR','/tmp/scyllascope-matplotlib')
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('summary');p.add_argument('output');a=p.parse_args()
r=json.loads(Path(a.summary).read_text());rows=[x for x in r['series'] if x['phase']!='final'];x=[s['elapsed_seconds']/60 for s in rows]
fig,axes=plt.subplots(3,1,figsize=(10,8),sharex=True,constrained_layout=True)
axes[0].plot(x,[s['rss_bytes']/2**20 for s in rows],label='RSS',color='#8059b3');axes[0].plot(x,[s['heap_alloc_bytes']/2**20 for s in rows],label='Go heap after GC',color='#147d92');axes[0].set_ylabel('Memory (MiB)');axes[0].legend(loc='upper right')
axes[1].plot(x,[s['heap_objects'] for s in rows],color='#147d92');axes[1].set_ylabel('Heap objects')
for key,label in [('queued','Queued'),('inflight','In flight'),('tracks','Counters')]:axes[2].plot(x,[s['alerts'][key] for s in rows],label=label)
axes[2].set_ylabel('Resource count');axes[2].set_xlabel('Elapsed real time (minutes)');axes[2].legend(loc='upper right')
for ax in axes:
 ax.axvspan(0,r['warmup_seconds']/60,color='#eeeeee',label='Warmup');ax.grid(alpha=.2);ax.set_xlim(left=0)
fig.suptitle('ScyllaScope: finite collector / SQLite / notification / SSE workload\nSynthetic loopback fixtures; slopes do not prove resolution of a weeks-long issue',fontsize=12)
fig.savefig(a.output,dpi=160,metadata={'Creator':'ScyllaScope validation'});plt.close(fig)
