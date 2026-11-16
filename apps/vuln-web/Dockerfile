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
RUN apk upgrade --no-cache && rm /etc/nginx/conf.d/default.conf && mkdir -p /etc/nginx/snippets
COPY --from=build /src/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/vuln-web.conf
COPY docker/security.conf /etc/nginx/snippets/security.conf
USER 101:101
EXPOSE 8080
