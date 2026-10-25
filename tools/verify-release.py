#!/usr/bin/env python3
"""Verify prepared real assets without installing or operating any service."""
import argparse
import hashlib
from pathlib import Path
import struct
import subprocess
import tarfile
import tempfile

ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory');p.add_argument('--sentinel');a=p.parse_args()
root=Path(a.directory);version=(root/'VERSION').read_text().strip();tag='v'+version
assets={f'scyllascope-{tag}-linux-{arch}.tar.gz' for arch in ('amd64','arm64')}|{'VERSION','install.sh'}
lines=(root/'SHA256SUMS').read_text().splitlines();checks={}
for line in lines:
 digest,name=line.split()
 assert name in assets and name not in checks and len(digest)==64,'manifest member/duplicate'
 assert hashlib.sha256((root/name).read_bytes()).hexdigest()==digest,'checksum mismatch'
 checks[name]=digest
assert set(checks)==assets,'incomplete checksum manifest'
members={'scyllascope','VERSION','LICENSE','THIRD_PARTY.md','LICENSES/RC-MIT.txt'}
for arch,machine in [('amd64',62),('arm64',183)]:
 with tarfile.open(root/f'scyllascope-{tag}-linux-{arch}.tar.gz','r:gz') as archive:
  entries=archive.getmembers();assert len(entries)==5 and {m.name for m in entries}==members
  assert all(m.isfile() for m in entries),'non-regular release member'
  body={m.name:archive.extractfile(m).read() for m in entries}
 assert body['VERSION']==(version+'\n').encode()
 assert body['LICENSE']==(ROOT/'LICENSE').read_bytes()
 assert body['LICENSES/RC-MIT.txt']==(ROOT/'LICENSES/RC-MIT.txt').read_bytes()
 notice=body['THIRD_PARTY.md'].decode();assert 'Exact license texts for this prebuilt distribution' in notice
 for text in ['Go runtime and standard library','github.com/scylladb/gocql','modernc.org/sqlite','FrontEnd'.lower()]:
  assert text.lower() in notice.lower(),'missing upstream distribution notice'
 binary=body['scyllascope'];assert binary[:6]==b'\x7fELF\x02\x01' and struct.unpack_from('<H',binary,18)[0]==machine
 offset=struct.unpack_from('<Q',binary,32)[0];size,count=struct.unpack_from('<HH',binary,54)
 assert all(struct.unpack_from('<I',binary,offset+i*size)[0]!=3 for i in range(count)),'dynamic interpreter unexpectedly required'
 if a.sentinel:assert a.sentinel.encode() not in binary,'untracked build data was embedded'
 if arch=='amd64':
  with tempfile.TemporaryDirectory(prefix='scyllascope-binary-version-') as directory:
   exe=Path(directory)/'scyllascope';exe.write_bytes(binary);exe.chmod(0o700)
   result=subprocess.run([str(exe),'version'],capture_output=True,text=True,check=True,timeout=10)
   assert result.stdout.strip()=='ScyllaScope '+version
 print(arch+': static Linux ELF, exact license/version/member/hash verified')
print('No installation, service operation, tag or Release publication performed.')
