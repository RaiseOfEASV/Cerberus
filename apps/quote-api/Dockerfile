# Build stage: plain public toolchain image. docker/base/README.md's
# central-base rule only applies to the final runtime stage below.
FROM golang:1.26-alpine AS build
WORKDIR /src
# go.sum* - optional; quote-api has no third-party deps (stdlib only) today.
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/quote-api ./cmd/quote-api

# Runtime stage: central distroless base (docker/base/go). Not digest-pinned
# yet - base-images.yml hasn't published one; Renovate pins it once it exists.
FROM ghcr.io/raiseofeasv/base-go:latest
COPY --from=build /out/quote-api /quote-api
# Numeric UID (distroless "nonroot") so Kubernetes' runAsNonRoot can verify
# it - a named user can't be checked at admission time.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/quote-api"]
