#!/usr/bin/env python3
"""Combine allowlisted isolation runs, retaining every failed attempt.
Inputs are private run roots, ordered by execution; no paths enter the export.
"""
import argparse
import copy
import json
from pathlib import Path
from analyze_isolation import analyze

ORDER=['idle','sqlite_fixed','collector_fixed','json_only','json_sse','notification','sqlite_compact']
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('roots',nargs='+');p.add_argument('--output',required=True);a=p.parse_args()
runs=[analyze(Path(root)) for root in a.roots]
selected={};prior=[];run_records=[]
for index,run in enumerate(runs):
 record={k:v for k,v in run.items() if k!='cases'}
 record['run_index']=index;record['case_records']=[]
 for case in run['cases']:
  c=copy.deepcopy(case)
  if c['manifest_state']=='not_run':continue
  c['run_index']=index;c['source_commit']=run['source_commit'];c['binary_sha256']=run['binary_sha256']
  record['case_records'].append({k:c.get(k) for k in ('mode','manifest_state','manifest_exit_code','started_utc','finished_utc','planned_duration_seconds','observed_manifest_wall_seconds','samples_total','analysis_issues')})
  mode=c['mode']
  if mode in selected:prior.append(selected[mode])
  selected[mode]=c
 run_records.append(record)
cases=[selected[mode] for mode in ORDER if mode in selected]
result={'schema_version':1,'run_records':run_records,'selected_cases':cases,'prior_attempts':prior,
        'all_required_cases_present':len(cases)==len(ORDER),
        'all_selected_cases_completed_successfully':len(cases)==len(ORDER) and all(c['case_completed_successfully'] for c in cases),
        'any_failed_attempt':any(c['manifest_state'] in ('failed','degraded') for c in cases+prior),
        'memory_acceptance':'undetermined',
        'limitations':['Collector uses diagnostic-only test changes; production sources unchanged. See per-case source and binary hashes.',
                       'Original SQLite full and compact cases must have identical binary hashes for their direct comparison.',
                       'All initial failures remain visible; later successful work does not resolve an earlier unexplained failure.',
                       'A finite component study does not establish resolution of week-scale growth.']}
full=selected.get('sqlite_fixed',{});compact=selected.get('sqlite_compact',{})
result['sqlite_comparison_same_binary']=bool(full.get('binary_sha256')) and full.get('binary_sha256')==compact.get('binary_sha256')
Path(a.output).write_text(json.dumps(result,ensure_ascii=False,indent=2,allow_nan=False)+'\n')
