# Build stage: plain public node image - base-node is distroless (no shell,
# no npm), and docker/base/README.md's central-base rule only covers an
# app's final runtime stage.
FROM node:26-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Runtime stage: nginx stable line, unprivileged variant -
# listens on 8080 and runs as UID 101 out of the box.
FROM nginxinc/nginx-unprivileged:1.30-alpine
USER root
# apk upgrade: pick up Alpine security fixes published after the upstream
# image was built (build.yml's Trivy gate fails on fixed CRITICALs).
RUN apk upgrade --no-cache \
    && rm /etc/nginx/conf.d/default.conf \
    && mkdir -p /run/quote-web \
    && chown 101:101 /run/quote-web
COPY --from=build /src/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/quote-web.conf
COPY --chmod=0755 docker/docker-entrypoint.sh /docker-entrypoint.sh
# The committed dev fallback must not ship - the entrypoint writes the real
# one under /run/quote-web at start-up.
RUN rm -f /usr/share/nginx/html/env-config.js
USER 101:101
EXPOSE 8080
ENTRYPOINT ["/docker-entrypoint.sh"]
