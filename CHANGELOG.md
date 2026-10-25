# 0.6.3 — 2026-10-25

- Import the supplied 0.6.3-rc.2 source while preserving the repository LICENSE and original RC notice. Remove operational examples, raw review logs, internal addresses and identity fixtures.
- Retire counters for unavailable node identities without falsely resolving incidents.
- Separate paged notification work and ID lookups from the 500-row display limit; persist unsent opening/recovery work, retain it during pruning and rotate full notification queues fairly.
- Add Alertmanager grouping, independent protocol conditions, periodic refresh and all-endpoint delivery for HA deployments.
- Distribute prebuilt Linux AMD64/ARM64 archives with SHA-256 verification and one-command install/update/uninstall; retain configuration/data by default and reuse automatic failure rollback.
- Preserve the unresolved long-term RSS, native systemd and ARM64 acceptance limits in docs/RELEASE-NOTES.md.
- Add verified-build installer; include secrets.env in new rollback manifests while reading older manifests without changing that file.
- Keep hidden topology layouts from resetting zoom centers; strengthen browser filtering assertions.
- Resolve the real dependency graph and add reproducible SQLite/HTTP soaks and real Alertmanager checks. See VALIDATION.md for measured scope and outstanding release gates.

# 0.6.3-rc.2 — 2026-10-16

Final-review fixes; **still not a formal/production-approved release**.

- Cancel outstanding resource requests before serving a cached selection, preventing late history responses from replacing the selected range.
- Invalidate stale build verification markers on every attempted build, including failed toolchain checks; refuse skipped race tests.
- Add versioned backup manifests with streamed SHA-256 and size checks. Validate every new-format backup before modifying any destination. Legacy manifests remain supported with an explicit integrity warning.
- Add regression tests reproducing all three issues. Original RC evidence is intentionally excluded from the public import; see docs/RC-REVIEW.md.

# 0.6.3-rc.1 — 2026-10-16

Memory/publication, bounded caches, cancellation and deployment error handling improvements. Historical RC status: the full build was blocked in its original environment. See docs/RC-REVIEW.md and VALIDATION.md for current validation.

The following is the previous release history. Any older validation statements below are not verification evidence for this candidate.

# Changelog

## 0.6.2

### Directional alert correctness
- Resolve orphaned asymmetric incidents when a fresh Scylla discovery proves that the old target `host_id` no longer exists (for example after node replacement / re-bootstrap).
- Never treat a missing target as recovery while discovery is failed, stale, or otherwise untrustworthy.
- Resolve an asymmetric incident when both directions become fresh and agree, even when they agree on failure; a symmetric outage is tracked separately and no longer keeps the asymmetry alert open forever.
- Added regression tests for retired host IDs, discovery-loss safety, and symmetric-failure recovery.

### Validation
- `go test` passed for alerts/core/security/settings/probe/monitor packages.
- `go vet` passed for the same dependency-free package set.
- Full-module tests still require external Go modules; this offline build environment cannot reach `proxy.golang.org`.

## 0.6.1

### Build / portability fix
- Fixed an environment-dependent config test that incorrectly used the host's live Tailscale IP while validating a temporary Scylla fixture.
- `go test ./...` is now deterministic across different CPU architectures and host configurations even when each host has a different Tailscale address.
- Runtime auto-detection behavior is unchanged; this is a testability/build-gate fix.

## 0.6.0

### SPA / usability
- Replaced page-wide string/innerHTML rendering with actual Svelte 5 compiled components.
- Keyed nodes, edges and table rows; one shared SSE connection.
- Preserve live-update focus/selection, native details, drawer/tab/filter state, chart range,
  query text/result, topology viewport and per-page scroll.
- Compact desktop overview; readable mobile node list and dedicated vertical graph.
- Local assets only, explicit cache retirement for old service workers, gzip/ETag.
- Real metrics history ranges, one-minute refresh only while visible, missing-value gaps.

### Correctness / safety
- Preserve Unknown/stale/future distinctions and directed evidence provenance.
- Fix cold-start alerts, recovery retries, recurrence identity and deferred ack/snooze handling.
- Session-specific immediate revocation, bounded SSE/query/login resources, secret redaction.
- Strict read-only CQL grammar with table allowlist and a database-side LIMIT.
- Hardened unprivileged systemd installer, ownership-preserving rollback and update lock.
- Fail-fast full-build script that does not replace a running installation on test failure.
- No destructive Scylla restart/repair/drop actions or fabricated replica health.

### Verification
- 53 backend subset tests with race detection; 10 frontend function tests.
- 21 real-browser API/SSE regressions; detailed limits and evidence in VALIDATION.md.
- Full external Go dependency resolution / production linking was unavailable here.

---
## Earlier release notes (historical claims, see current VALIDATION.md)

# Changelog

## 0.5.1-preview

- Dashboard density redesigned: smaller sidebar, header, KPI cards, topology and tables.
- Dashboard now focuses on topology, active alerts and node health; history and connection matrix remain on dedicated pages.
- Removed zoom controls from the mini topology card.
- Compact monitoring-coverage and warning presentation.
- Reduced topology whitespace for 1–8 node clusters.
- Fixed sub-millisecond CQL latency sampling stability (`0.1 ms` stays `0.1`, not floating-point noise).

## v0.4.0

- Made navigation fully client-side and removed API fetches caused only by page changes.
- Initial snapshot/config/events/history now load in parallel.
- Decoupled event polling from the fast SSE metric stream.
- Reduced hidden-page DOM work and debounced topology resize renders.
- Added persistent SQLite metric samples and `/api/v1/history`.
- Added PWA manifest, service worker and mobile app icons.
- Reworked topology identity around Scylla host IDs.
- Merges directional edge observations from peer ScyllaScope instances.
- Added asymmetric-link visualization foundation and directional link details.
- Added direct lightweight scraping of Scylla's native `:9180/metrics` endpoint; no Prometheus server required.
- Added active/pending compaction, compaction backlog, read-overload and Raft-drop metrics.
- Added local Scylla runtime `--smp`, `--memory`, and actual `scylla --version` detection.
- Fixed `system.cluster_status.load` parsing to tolerate Scylla's text/numeric representations.
- Preserves the previous topology during temporary CQL outages instead of making nodes disappear.
- Added Tailscale host-name resolution for discovered cluster addresses.
- Added alert failure-window debouncing and recovery events.
- Prevented alert spam on daemon startup.
- Added `scyllascope doctor` diagnostics.
- Added on-demand browser diagnostics on the Settings page via `/api/v1/doctor`.
- Added `service restart` and `service logs` helpers.
- Made service installation/update atomic and restart onto the new executable.
- Stopped generating a duplicate full Scylla config during service install; live `/etc/scylla/scylla.yaml` remains the source of truth.
- `scyllascope init` now writes a minimal override template.
- Added `scyllascope config reset` for v0.3 migration / returning to pure auto-detection.
- Updated the Scylla Go driver replacement to v1.19.0.
- Added unit tests for monitoring parsers and config override behavior.

## v0.3.0

- Added zero-config discovery from `/etc/scylla/scylla.yaml`.
- Added `detect`, `init`, and systemd install/status/uninstall helpers.

## v0.2.0

- Replaced the static dashboard mock with interactive pages and live backend actions.
- Added manual refresh, settings endpoint, events clear, read-only CQL console and live charts.

## v0.1.0

- Initial dashboard prototype and distributed health monitor foundation.
