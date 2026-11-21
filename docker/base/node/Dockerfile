# Central runtime base for Node/JS-TS apps. Distroless: no shell, no package
# manager, just the Node runtime. App images COPY their built output
# (node_modules + dist/build) onto this. Renovate tracks the digest below.
FROM gcr.io/distroless/nodejs26-debian13:nonroot@sha256:1d736cf8b5716c07a245301f92717143215d248fd55771ee1936ef4b3302e531

USER nonroot:nonroot
