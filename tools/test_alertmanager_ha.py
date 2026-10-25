#!/usr/bin/env python3
"""Isolated real two-process Alertmanager HA stop/rejoin integration test.

Only loopback listeners and synthetic incidents are used. Private temporary
process logs and receiver payloads are deleted on exit; stdout is a summary.
"""
import argparse
import datetime
import http.server
import json
import pathlib
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request


def free_port(allocated):
    while True:
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            number = sock.getsockname()[1]
            if number in allocated:
                continue
            # Gossip needs UDP as well as TCP at the selected address.
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as datagram:
                try:
                    datagram.bind(('127.0.0.1', number))
                except OSError:
                    continue
            allocated.add(number)
            return number


def main(binary, output=None):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    notices, lock = [], threading.Lock()

    class Receiver(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            payload = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            with lock:
                notices.append(payload)
            self.send_response(200)
            self.end_headers()

        def log_message(self, *_args):
            pass

    receiver = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Receiver)
    thread = threading.Thread(target=receiver.serve_forever, daemon=True)
    thread.start()
    with tempfile.TemporaryDirectory(prefix='scyllascope-am-ha-') as temporary:
        root = pathlib.Path(temporary)
        config = root/'config.yaml'
        config.write_text(f'''route:
  receiver: synthetic
  group_by: [alertname, cluster, node, kind]
  group_wait: 1s
  group_interval: 1s
  repeat_interval: 1h
receivers:
- name: synthetic
  webhook_configs:
  - url: http://127.0.0.1:{receiver.server_port}
    send_resolved: true
''')
        allocated = {receiver.server_port}
        web = [free_port(allocated), free_port(allocated)]
        gossip = [free_port(allocated), free_port(allocated)]
        processes, logs = [None, None], []
        start_time = datetime.datetime.now(datetime.timezone.utc)

        def get(index, path):
            with opener.open(f'http://127.0.0.1:{web[index]}{path}', timeout=2) as response:
                return json.load(response) if path.startswith('/api/') else response.read()

        def wait(predicate, message, seconds=25):
            deadline = time.monotonic()+seconds
            while time.monotonic() < deadline:
                try:
                    if predicate():
                        return
                except (OSError, ValueError):
                    pass
                time.sleep(.1)
            raise AssertionError(message)

        def start(index):
            log = open(root/f'process-{index}.log', 'a')
            logs.append(log)
            processes[index] = subprocess.Popen([
                binary, f'--config.file={config}', f'--storage.path={root}/data-{index}',
                f'--web.listen-address=127.0.0.1:{web[index]}',
                f'--web.external-url=http://127.0.0.1:{web[index]}',
                f'--cluster.listen-address=127.0.0.1:{gossip[index]}',
                f'--cluster.advertise-address=127.0.0.1:{gossip[index]}',
                f'--cluster.peer=127.0.0.1:{gossip[1-index]}',
                '--cluster.peer-timeout=2s', '--cluster.settle-timeout=3s',
                '--cluster.pushpull-interval=1s', '--cluster.reconnect-interval=1s',
            ], stdout=log, stderr=log)
            wait(lambda: get(index, '/-/ready'), 'process failed to become ready')

        def stop(index):
            process = processes[index]
            if process is not None and process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
            processes[index] = None

        def clustered():
            return all(len(get(i, '/api/v2/status')['cluster']['peers']) == 2 for i in range(2))

        def alert(observer, kind='node_health/cql', resolved=False):
            now = datetime.datetime.now(datetime.timezone.utc)
            return {
                'labels': {'alertname': 'ScyllaScopeIncident', 'cluster': 'synthetic-ha',
                           'node': 'demo', 'kind': kind, 'observer': observer,
                           'incident_id': observer+':'+kind+':episode-1'},
                'annotations': {'summary': 'synthetic HA integration'},
                'startsAt': start_time.isoformat(),
                'endsAt': (now if resolved else now+datetime.timedelta(minutes=5)).isoformat(),
            }

        def send(index, alerts):
            request = urllib.request.Request(f'http://127.0.0.1:{web[index]}/api/v2/alerts',
                                             data=json.dumps(alerts).encode(),
                                             headers={'Content-Type': 'application/json'})
            with opener.open(request, timeout=3):
                pass

        def matching(kind, status=None):
            with lock:
                return [x for x in notices if x['groupLabels']['kind'] == kind and
                        (status is None or x['status'] == status)]

        try:
            start(0)
            start(1)
            wait(clustered, 'two processes did not form a two-member cluster')
            # Group wait must start after both receivers are ready, rather than
            # use a timestamp from before process startup.
            start_time = datetime.datetime.now(datetime.timezone.utc)
            original = [alert('observer-a'), alert('observer-b')]
            send(0, original)
            send(1, original)
            wait(lambda: matching('node_health/cql', 'firing'), 'initial firing not delivered')
            time.sleep(3)
            initial = matching('node_health/cql', 'firing')
            assert len(initial) == 1, 'healthy HA pair duplicated initial group'
            assert len(initial[0]['alerts']) == 2, 'two observers were not grouped'
            # Stop the member that actually delivered the notification, so the
            # test exercises notification-owner loss rather than arbitrary loss.
            sender_url = initial[0]['externalURL'].rstrip('/')
            owner = next(i for i in range(2) if sender_url == f'http://127.0.0.1:{web[i]}')
            survivor = 1-owner
            stop(owner)
            send(survivor, [alert('observer-a', kind='node_health/internode')])
            wait(lambda: matching('node_health/internode', 'firing'), 'survivor lost distinct failure')
            send(survivor, [alert('observer-a', resolved=True)])
            time.sleep(3)
            assert not matching('node_health/cql', 'resolved'), 'partial recovery resolved whole group'
            send(survivor, [alert('observer-b', resolved=True)])
            wait(lambda: matching('node_health/cql', 'resolved'), 'survivor lost complete recovery')
            send(survivor, [alert('observer-a', kind='node_health/internode', resolved=True)])
            wait(lambda: matching('node_health/internode', 'resolved'), 'survivor lost second recovery')
            time.sleep(3)
            with lock:
                before_restart = len(notices)
            start(owner)
            wait(clustered, 'restarted process did not rejoin cluster')
            settled = [alert('observer-a', resolved=True), alert('observer-b', resolved=True),
                       alert('observer-a', kind='node_health/internode', resolved=True)]
            for index in range(2):
                send(index, settled)
            time.sleep(5)
            with lock:
                assert len(notices) == before_restart, 'rejoin/resend repeated completed notifications'
            # Confirm the rejoined cluster can process a new kind, not merely
            # remain quiet after restart with no useful work.
            for index in range(2):
                send(index, [alert('observer-a', kind='node_health/service')])
            wait(lambda: matching('node_health/service', 'firing'), 'rejoined cluster lost new work')
            result = {'result': 'PASS', 'alertmanager_version': '0.28.1',
                      'processes': 2, 'initial_firing_notifications': len(initial),
                      'post_restart_quiet_window_seconds': 5,
                      'test_timers_seconds': {'peer_timeout': 2, 'settle_timeout': 3,
                                             'pushpull_interval': 1, 'reconnect_interval': 1,
                                             'group_wait': 1, 'group_interval': 1},
                      'checks': ['two actual processes form HA cluster',
                                 'two observers grouped with one initial notification',
                                 'actual notification sender stops',
                                 'survivor delivers distinct failure and both recoveries',
                                 'partial recovery remains firing',
                                 'persistent-storage restart rejoins cluster',
                                 'resolved replay after rejoin produces no duplicate',
                                 'rejoined cluster processes new work'],
                      'limitations': ['graceful member stop only; not SIGKILL or disk loss',
                                      'loopback transport with shortened HA timers',
                                      'no real network partition, packet loss or clock skew',
                                      'no exactly-once delivery guarantee'],
                      'notifications': sum(len(matching(kind)) for kind in
                                           ['node_health/cql','node_health/internode','node_health/service'])}
            rendered = json.dumps(result, indent=2)+'\n'
            if output:
                pathlib.Path(output).write_text(rendered)
            print(rendered, end='')
        finally:
            for i in range(2):
                stop(i)
            for log in logs:
                log.close()
            receiver.shutdown()
            receiver.server_close()
            thread.join(timeout=5)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary')
    parser.add_argument('--output', help='Write deidentified summary JSON')
    arguments = parser.parse_args()
    main(arguments.binary, arguments.output)
