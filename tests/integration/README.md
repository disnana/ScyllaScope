# Disposable local Scylla integration

This suite is opt-in. It rejects non-loopback seed addresses and requires a
Docker container with a `scyllascope-isolated-` name and the exact label
`scyllascope.integration=isolated`. Before creating a CQL client/session, it also checks the fixture's `9042/tcp`
published bindings: every HostIP must be a literal loopback address, and the
selected seed IP and port must match one of those bindings. A labelled fixture
therefore cannot authorize a seed pointing at a different local CQL server.
Twelve pure-function regression cases cover matching IPv4/IPv6, wrong local
ports/addresses, wildcard/mixed-public bindings, missing/wrong container ports,
invalid inspection JSON and invalid endpoints. These run in the default suite.

It reads that fixture's network identity to
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

The plaintext fixture does not test authentication or TLS. The separate
opt-in authenticated TLS case below covers those policies. Cancellation during
initial session creation, multi-node topology, version compatibility beyond
6.2.3, and long-running CQL allocation behavior remain acceptance conditions.
Raw Docker/CQL logs are private temporary artifacts and are not committed.

After this guard was added, the real fixture race suite was repeated successfully
(10.896 seconds, no race), and its container was removed again.


## Separate authenticated TLS fixture

With the same image already cached, run:

```bash
SCYLLASCOPE_AUTH_TLS_INTEGRATION=1 \
  go test -race -timeout=5m -count=1 -v ./tests/integration \
  -run '^TestDisposableScyllaAuthenticationTLS$'
```

This case provisions its own uniquely named, labelled disposable local container
and checks its published loopback CQL port before creating the CQL client.
It generates a private temporary CA, RSA leaf certificate/key and dedicated
Scylla YAML inside `t.TempDir`; they are read-only mounted into the fixture and
removed after the test. The certificate SAN contains only the synthetic
`scylla.fixture.invalid` name and the loopback IP. No host trust store is changed.
The Scylla image's documented default superuser credentials are used solely in
this brand-new fixture; operational credentials are never read.

The test uses PasswordAuthenticator and CassandraAuthorizer with client TLS on
9042. The production adapter succeeds with the trusted fixture CA and matching
synthetic hostname. Wrong password, missing credentials, wrong CA, and wrong
hostname are each rejected. A final valid connection succeeds after those
negative cases to check that the fixture stayed healthy.

The final 2026-10-25 race execution passed in 7.519 seconds (one top-level case and
four negative subtests), with no race. Driver diagnostics confirmed password
rejection, authentication-required rejection, unknown-authority rejection, and
SAN hostname mismatch. An earlier independent run also passed in 13.253 seconds. Each temporary fixture
was removed and absence confirmed.
Mutual TLS/client certificates, revocation/rotation, other Scylla versions,
multi-node authentication/topology and long-term CQL memory behavior are not
covered by this fixture.
