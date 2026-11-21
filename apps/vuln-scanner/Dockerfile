# Nightly Trivy scanner (see cmd/vuln-scanner). The Trivy binary comes from
# the official image, pinned by digest - bump it deliberately (CI fails the
# build on a CRITICAL CVE in it, as it did on 0.69.3).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vuln-scanner ./cmd/vuln-scanner

FROM aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb AS trivy

# Runtime stage: central distroless base (docker/base/go) - CA certificates
# for the registry and the Trivy database download, nothing else.
FROM ghcr.io/raiseofeasv/base-go:latest
COPY --from=build /out/vuln-scanner /vuln-scanner
COPY --from=trivy /usr/local/bin/trivy /trivy
USER 65532:65532
ENTRYPOINT ["/vuln-scanner"]
