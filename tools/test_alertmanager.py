#!/usr/bin/env python3
"""Optional real Alertmanager grouping/restart test; local synthetic data only."""
import datetime, http.server, json, os, pathlib, socket, subprocess, sys, tempfile, threading, time, urllib.request

def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0)); return s.getsockname()[1]

def main(binary):
    notifications = []
    started = datetime.datetime.now(datetime.timezone.utc)
    class Receiver(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            notifications.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
            self.send_response(200); self.end_headers()
        def log_message(self, *args): pass
    receiver = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Receiver)
    threading.Thread(target=receiver.serve_forever, daemon=True).start()
    with tempfile.TemporaryDirectory() as temp:
        root = pathlib.Path(temp); config = root/'config.yaml'; p = port()
        config.write_text(f'''route:
  receiver: test
  group_by: [alertname, cluster, node, kind]
  group_wait: 1s
  group_interval: 1s
  repeat_interval: 1h
receivers:
- name: test
  webhook_configs:
  - url: http://127.0.0.1:{receiver.server_port}
    send_resolved: true
''')
        log = open(root/'process.log', 'w')
        def start():
            proc = subprocess.Popen([binary, f'--config.file={config}', f'--storage.path={root}/data', f'--web.listen-address=127.0.0.1:{p}', '--cluster.listen-address='], stdout=log, stderr=log)
            for _ in range(100):
                try:
                    urllib.request.urlopen(f'http://127.0.0.1:{p}/-/ready', timeout=1).close(); return proc
                except OSError: time.sleep(.1)
            proc.terminate(); proc.wait(); raise AssertionError('Alertmanager not ready')
        def send(observer, resolved=False, kind='node_health/cql'):
            now = datetime.datetime.now(datetime.timezone.utc)
            alert = {'labels': {'alertname':'ScyllaScopeIncident','cluster':'synthetic','node':'demo','kind':kind,'observer':observer}, 'startsAt':started.isoformat(), 'endsAt':(now+datetime.timedelta(minutes=5) if not resolved else now).isoformat()}
            req = urllib.request.Request(f'http://127.0.0.1:{p}/api/v2/alerts', data=json.dumps([alert]).encode(), headers={'Content-Type':'application/json'})
            urllib.request.urlopen(req, timeout=2).close()
        def wait_for(predicate):
            for _ in range(100):
                if predicate(): return
                time.sleep(.1)
            raise AssertionError(json.dumps(notifications))
        proc = start()
        try:
            send('a'); send('b')
            wait_for(lambda: len(notifications)>=1)
            assert len(notifications)==1 and len(notifications[0]['alerts'])==2, json.dumps(notifications)
            send('a', True); time.sleep(2)
            assert not any(x['status']=='resolved' for x in notifications), 'one reporter resolved the whole group'
            # Distinct conditions create a separate notification group.
            send('a', kind='node_health/internode')
            wait_for(lambda: any(x['groupLabels']['kind']=='node_health/internode' for x in notifications))
            send('b', True)
            wait_for(lambda: any(x['status']=='resolved' and x['groupLabels']['kind']=='node_health/cql' for x in notifications))
            before = len(notifications)
            proc.terminate(); proc.wait(timeout=10); proc = start()
            send('a', kind='node_health/internode'); time.sleep(3)
            assert len(notifications)==before, 'restart repeated a delivered notification'
            print(json.dumps({'result':'PASS','checks':['two observers grouped','partial recovery stays firing','different conditions separated','complete recovery','persistent notification log across restart'],'notifications':len(notifications)}))
        finally:
            proc.terminate(); proc.wait(timeout=10); receiver.shutdown(); log.close()
if __name__=='__main__': main(sys.argv[1])
