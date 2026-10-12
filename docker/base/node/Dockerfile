# Central runtime base for Node/JS-TS apps. Distroless: no shell, no package
# manager, just the Node runtime. App images COPY their built output
# (node_modules + dist/build) onto this. Renovate tracks the digest below.
FROM gcr.io/distroless/nodejs22-debian12:nonroot

USER nonroot:nonroot
