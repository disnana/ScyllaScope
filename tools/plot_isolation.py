#!/usr/bin/env python3
"""Plot the public, allowlisted combined isolation summary; never reads raw logs."""
import argparse
import json
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('summary');p.add_argument('output');a=p.parse_args()
r=json.load(open(a.summary));cases=r['selected_cases']+r.get('confirmations',[])
fig,axes=plt.subplots(4,2,figsize=(12,12),constrained_layout=True)
metrics=[('rss_bytes','RSS','#1f77b4'),('go_sys_bytes','Go Sys','#ff7f0e'),('heap_alloc_bytes','HeapAlloc','#2ca02c'),('allocator_mapped_bytes','native mapped','#d62728')]
for ax,c in zip(axes.flat,cases):
 rows=[x for x in c['sample_series'] if x['phase']!='post_stop_gc']
 for key,label,color in metrics:
  points=[x for x in rows if key in x]
  ax.plot([x['elapsed_seconds']/60 for x in points],[x[key]/(1<<20) for x in points],label=label,color=color,linewidth=1)
 ax.axvspan(0,c['planned_warmup_seconds']/60,color='gray',alpha=.13)
 ax.set_title(c.get('role',c['mode'])+' — '+c['manifest_state'],fontsize=10)
 ax.set_xlabel('elapsed minutes');ax.set_ylabel('MiB');ax.grid(alpha=.2)
 ax.set_ylim(bottom=0)
axes.flat[0].legend(fontsize=8,ncol=2)
for ax in list(axes.flat)[len(cases):]:
 ax.axis('off')
 ax.text(0,.95,'Natural GC; gray = warmup.\nPost-stop GC excluded.\nExternal client/receiver RSS separate.\nCollector uses diagnostic-only binary.\nFixed state; amplified SQL Save + Load.\nInitial failed collector case retained separately.\nFinite observations do not establish a week-scale fix.',va='top',fontsize=10,linespacing=1.5)
fig.suptitle('ScyllaScope component isolation: measured test-process memory',fontsize=14)
fig.supxlabel('Gray: warmup; post-stop GC excluded. Collector incomplete. See report for limits.',fontsize=9)
fig.savefig(a.output,dpi=150,metadata={'Description':'Finite synthetic workload; see RSS-ISOLATION-RESULTS.md for limits.'})
