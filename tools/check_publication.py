#!/usr/bin/env python3
"""Check the git index for deployment material before publishing a branch.
Heuristics supplement manual review; they cannot prove absence of all secrets.
"""
import re, subprocess
from pathlib import PurePosixPath
paths = subprocess.check_output(['git','ls-files','-z']).decode().split('\0')
errors = []
for name in filter(None,paths):
    p=PurePosixPath(name)
    if p.suffix in {'.log','.jsonl','.db','.pem','.key'} or p.name in {'.env','secrets.env','ui-token'} or 'evidence' in p.parts:
        errors.append(name+': deployment/evidence file')
    body=subprocess.check_output(['git','show',':'+name])
    # Real imported network addresses were replaced with synthetic CGN fixtures.
    for ip in re.findall(rb'\b100\.(?:\d{1,3}\.){2}\d{1,3}\b',body):
        if ip not in {b'100.64.0.0',b'100.64.0.1',b'100.64.0.2',b'100.64.0.3'}:
            errors.append(name+': non-fixture CGN address')
    if re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{36}|xox[baprs]-[A-Za-z0-9-]{20,}',body):
        errors.append(name+': credential pattern')
if errors: raise SystemExit('\n'.join(errors))
print('PASS: indexed files contain no disallowed deployment/log files or matched credential/address patterns; manual review remains required.')
