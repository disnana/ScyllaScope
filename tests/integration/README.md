# Disposable local Scylla integration

This suite is opt-in. It rejects non-loopback seed addresses and requires a
Docker container with a `scyllascope-isolated-` name and the exact label
`scyllascope.integration=isolated`. It reads that fixture's network identity to
retain the production adapter's host filter when Docker forwards the CQL port.
It must never target an operational server or reuse an operational volume.

The 2026-10-25 run used the official ScyllaDB **6.2.3 amd64** image pinned to
`sha256:2366233080ba65622b9ed1b4588709141c4456efdf923809f012b3711b513006`.
The compressed image was 297,166,762 bytes. The runtime exposed only
500,000,000 bytes of physical memory to Seastar, so the initial 1GiB configuration
was rejected. The successful fixture used 384MiB, one shard, developer mode,
and epoll. This small configuration is for correctness tests, not a deployment
recommendation or a performance benchmark.

Provision only this disposable local fixture (the outer environment needs
Docker socket access). Preserve the platform's Docker proxy/CA configuration:

```bash
env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_TLS \
  -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH \
  docker --host=unix:///var/run/docker.sock run -d \
  --name scyllascope-isolated-cql \
  --label scyllascope.integration=isolated \
  --memory=768m --cpus=1 --pids-limit=256 \
  -p 127.0.0.1:19042:9042 \
  scylladb/scylla@sha256:2366233080ba65622b9ed1b4588709141c4456efdf923809f012b3711b513006 \
  --smp 1 --memory 384M --overprovisioned 1 --developer-mode 1 \
  --io-setup 0 --reactor-backend epoll

SCYLLASCOPE_INTEGRATION_ADDR=127.0.0.1:19042 \
SCYLLASCOPE_INTEGRATION_CONTAINER=scyllascope-isolated-cql \
  go test -race -timeout=5m -count=1 -v ./tests/integration

docker --host=unix:///var/run/docker.sock rm -f scyllascope-isolated-cql
```

The test exercises the real gocql fork and production adapter: system.local
query, system.cluster_status discovery, a pre-canceled context, query failure
while the fixture is stopped, reconnection with the same adapter after restart,
and rejection after Close. It does not write user tables.

Authentication, TLS/CA/hostname verification, cancellation during initial
session creation, multi-node topology, version compatibility beyond 6.2.3, and
long-running CQL allocation behavior remain separate acceptance conditions.
Raw Docker/CQL logs are private temporary artifacts and are not committed.
