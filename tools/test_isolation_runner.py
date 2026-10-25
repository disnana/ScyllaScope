#!/usr/bin/env python3
"""End-to-end exit/state contract: health NG must never become a green run."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RUNNER=Path(__file__).with_name('run_isolation.py')
FAKE='''#!/usr/bin/env python3
import json,os,pathlib,sys
mode=os.environ['SCYLLASCOPE_ISOLATION_MODE']
root=pathlib.Path(os.environ['SCYLLASCOPE_ISOLATION_OUTPUT'])
if mode=='collector_fixed' and FAULT=='resource':sys.exit(1)
rows=[{'mode':mode,'phase':'sample','probe_health_failed':FAULT=='health' and mode=='collector_fixed'},
      {'mode':mode,'phase':'end','probe_health_failed':False}]
if FAULT=='missing':rows=[]
(root/'samples.jsonl').write_text(''.join(json.dumps(row)+'\\n' for row in rows))
'''

class IsolationRunnerTests(unittest.TestCase):
    def run_case(self,fault):
        with tempfile.TemporaryDirectory(prefix='scyllascope-runner-contract-') as directory:
            root=Path(directory);binary=root/'fixture'
            binary.write_text(FAKE.replace("mode=os.environ",'FAULT='+repr(fault)+'\nmode=os.environ'));binary.chmod(0o700)
            output=root/'run'
            result=subprocess.run([sys.executable,str(RUNNER),str(binary),str(output),'--source-commit','a'*40,'--modes','collector_fixed,json_only','--smoke'],capture_output=True,text=True,timeout=10)
            return result.returncode,json.loads((output/'manifest.json').read_text())
    def test_health_failure_survives_recovery_and_other_cases_run(self):
        code,m=self.run_case('health')
        self.assertEqual(code,2);self.assertTrue(m['complete']);self.assertTrue(m['probe_health_failed'])
        self.assertEqual([x['state'] for x in m['cases']],['degraded','pass'])
        self.assertEqual(m['cases'][0]['probe_health_status'],'failed')
    def test_healthy_completion_is_green(self):
        code,m=self.run_case('none')
        self.assertEqual(code,0);self.assertTrue(m['complete']);self.assertFalse(m['probe_health_failed'])
        self.assertEqual(m['cases'][0]['probe_health_status'],'passed')
    def test_resource_failure_stops_remaining_work(self):
        code,m=self.run_case('resource')
        self.assertNotEqual(code,0);self.assertFalse(m['complete'])
        self.assertEqual([x['state'] for x in m['cases']],['failed'])
    def test_missing_end_telemetry_cannot_pass(self):
        code,m=self.run_case('missing')
        self.assertNotEqual(code,0);self.assertFalse(m['complete'])
        self.assertEqual(m['cases'][0]['state'],'failed')

if __name__=='__main__':unittest.main(verbosity=2)
