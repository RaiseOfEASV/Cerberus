# Build stage: plain public toolchain image (docker/base/README.md's
# central-base rule only applies to the final runtime stage).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vuln-api ./cmd/vuln-api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vuln-verify-report ./cmd/vuln-verify-report

# Runtime stage: central distroless base (docker/base/go).
FROM ghcr.io/raiseofeasv/base-go:latest
COPY --from=build /out/ /
# Numeric UID (distroless "nonroot") so runAsNonRoot can be verified.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/vuln-api"]
