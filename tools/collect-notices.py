#!/usr/bin/env python3
"""Build distribution notices from the actual production Go dependency graph.
Run after go mod verify. No runtime config, environment dump or module sources
are copied; only explicit upstream license/notice texts are aggregated.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[1]

def go(*args, target=None):
    env=dict(os.environ)
    if target:env.update(GOOS='linux',GOARCH=target,CGO_ENABLED='0',GOWORK='off',GOFLAGS='')
    return subprocess.check_output(['go',*args],cwd=ROOT,text=True,env=env)

def objects(body):
    decoder=json.JSONDecoder();index=0
    while index<len(body):
        while index<len(body) and body[index].isspace():index+=1
        if index==len(body):break
        value,index=decoder.raw_decode(body,index)
        yield value

def license_text(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size>2*(1<<20):
        raise SystemExit('Missing/non-regular/oversized upstream notice: '+path.name)
    return path.read_text(encoding='utf-8').rstrip()+'\n'

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',required=True);args=parser.parse_args()
    modules={}
    for target in ('amd64','arm64'):
        for package in objects(go('list','-mod=readonly','-deps','-json','./cmd/scyllascope',target=target)):
            module=package.get('Module')
            if not module or module.get('Main'):continue
            effective=module.get('Replace') or module
            modules[(effective['Path'],effective.get('Version',''))]=effective
    if not modules:raise SystemExit('No production module inventory; refusing incomplete notices')
    parts=[(ROOT/'THIRD_PARTY.md').read_text().rstrip(),
           '\n---\n\n# Exact license texts for this prebuilt distribution\n',
           'Production modules below come from go list -deps ./cmd/scyllascope; test tools are excluded. '
           'Root license/notice variants are retained even when an optional component is not linked.\n',
           '\n## Go runtime and standard library ('+go('env','GOVERSION').strip()+')\n',
           license_text(Path(go('env','GOROOT').strip())/'LICENSE')]
    for (name,version),module in sorted(modules.items()):
        directory=Path(module['Dir'])
        notices=sorted(p for p in directory.iterdir() if p.name.upper().startswith(('LICENSE','LICENCE','NOTICE','COPYING')))
        if not notices:raise SystemExit('No upstream notice found for '+name)
        parts.append('\n## '+name+' '+version+'\n')
        for path in notices:
            parts.extend(['\n### '+path.name+'\n',license_text(path)])
    for path in [ROOT/'web/vendor/svelte/LICENSE',*sorted((ROOT/'web/vendor/licenses').glob('*.txt'))]:
        parts.extend(['\n## Frontend notice: '+path.relative_to(ROOT).as_posix()+'\n',license_text(path)])
    output=Path(args.output);output.write_text('\n'.join(parts),encoding='utf-8')
    print('Collected Go runtime, '+str(len(modules))+' production modules and frontend license texts.')

if __name__=='__main__':main()
