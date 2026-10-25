# Third-party components

## Bundled Svelte

Svelte **5.48.0** is an actual dependency, not a simulated reactive renderer.
The compiler, runtime and package metadata are the locally available ESM
build distributed in Gradio's frontend assets. Files were copied without
modifying the Svelte implementation into `web/vendor/svelte/`.
`web/build.mjs` compiles application Svelte components using this compiler.
It does not fetch code from a CDN or replace Go modules with mocks.

Upstream: https://github.com/sveltejs/svelte / https://svelte.dev/
Bundled version is recorded in `svelte_package.json.js`; it is **not** a claim
that this is the newest upstream version. Svelte's MIT license is retained
in `web/vendor/svelte/LICENSE`, and in the compiled runtime assets.

The compiler distribution incorporates upstream utilities such as Acorn,
ARIA metadata, source-map tools, string/source transforms and tree walking.
Upstream comments/package metadata are retained. Acorn's license and the
Apache-2.0 license text are also included under `web/vendor/licenses/`.
This inventory is not an independently audited, exhaustive transitive SBOM;
organizations requiring one should regenerate/scan the dependency graph
and review the bundled compiler before external redistribution.

`web/vendor/SHA256SUMS` records the exact bundled compiler/runtime sources.
`internal/api/static/ui-manifest.sha256` records the compiled application
and runtime assets. Regenerating the frontend was checked for deterministic
output in this environment. No font files are included.

## Go dependencies

The source retains the previous version's explicit pins:

| Import / module | Pin | Purpose |
|---|---|---|
| github.com/gocql/gocql | v1.7.0 import, replaced below | CQL API import compatibility |
| github.com/scylladb/gocql | v1.19.0 via replace | Production Scylla fork |
| gopkg.in/yaml.v3 | v3.0.1 | Real YAML configuration parsing |
| modernc.org/sqlite | v1.59.0 | Pure-Go production SQLite driver |

Module source trees are not vendored in the source checkout. Prebuilt binaries
include the production Go dependencies statically. `./build.sh` resolves and
verifies modules, runs all tests, and creates the production binary.
`tools/collect-notices.py` appends the exact Go runtime/standard-library license,
production module root LICENSE/NOTICE/COPYING variants, and frontend notices to
this document inside each release archive. The archive therefore carries the
license texts as well as this overview; module sources and private configuration
are not copied into the distribution.
The production dependency graph was resolved and go.sum is committed.
Review `go list -m -json all` and `govulncheck ./...` when updating dependencies.
A successful build/checksum is not a vulnerability audit.

## Tests / tooling

Playwright and Chromium are external development/test tools, not bundled
with the runtime. Browser tests use the real HTTP API and Svelte UI with
clearly identified data-source fixtures. `cmd/devserver` is never compiled
into `cmd/scyllascope`.

Application icons are simple local SVG geometry. The topology renderer is
an application SVG component; no ELK/Cytoscape package is represented as
installed when it is not. Native SVG suffices for the bounded small graph
implemented in this release.

## Imported ScyllaScope RC

The original RC MIT notice, Copyright (c) 2026 ScyllaScope contributors,
is retained in `LICENSES/RC-MIT.txt`. The repository's existing `LICENSE`
(Copyright (c) 2026 Disnana) remains byte-for-byte unchanged.
