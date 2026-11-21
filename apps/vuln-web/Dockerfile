# Build stage: plain public node image (the central-base rule only covers
# the final runtime stage).
FROM node:26-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Runtime: nginx stable, unprivileged variant - listens on 8080 as UID 101.
FROM nginxinc/nginx-unprivileged:1.30-alpine
USER root
# apk upgrade: Alpine fixes published after the upstream image was built
# (build.yml's Trivy gate fails on fixed CRITICALs).
# The base image pins nginx and its optional modules to one exact version,
# so `apk upgrade` alone can't take an nginx security fix. vuln-web loads
# none of the modules: drop them, then take the newest nginx in the repos.
RUN apk upgrade --no-cache \
 && apk del --no-cache nginx-module-geoip nginx-module-image-filter nginx-module-njs nginx-module-xslt \
 && apk add --no-cache --upgrade nginx \
 && rm /etc/nginx/conf.d/default.conf && mkdir -p /etc/nginx/snippets
COPY --from=build /src/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/vuln-web.conf
COPY docker/security.conf /etc/nginx/snippets/security.conf
USER 101:101
EXPOSE 8080
