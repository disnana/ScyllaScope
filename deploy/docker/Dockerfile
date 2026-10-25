# Alternative packaging; native systemd is recommended for local Scylla access.
# Override GO_IMAGE with a tested, digest-pinned toolchain image in your CI.
ARG GO_IMAGE=golang:1.27.3-bookworm
FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY . .
RUN go mod tidy && go mod verify && go test -count=1 ./... && go vet ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scyllascope ./cmd/scyllascope

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 scyllascope \
 && useradd -r -u 10001 -g 10001 -d /var/lib/scyllascope -s /usr/sbin/nologin scyllascope \
 && mkdir -p /var/lib/scyllascope && chown 10001:10001 /var/lib/scyllascope
COPY --from=build /out/scyllascope /usr/local/bin/scyllascope
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/scyllascope"]
# Mount the real Scylla YAML + token + overrides read-only, state read-write.
# No Docker socket mount and no privileged mode. systemd checks will be unavailable.
