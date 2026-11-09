# quote-web

React + TypeScript (Vite) frontend for cerberoast.site. Fetches a random
quote from `quote-api` and displays it.

The API base URL is **not** baked into the build: `public/env-config.js` is a
local-dev fallback, and the container overwrites it at start-up from the
`API_URL` environment variable (`docker/docker-entrypoint.sh`), so one image
is promoted unchanged across dev, staging and prod.

`just build` / `just test` / `just lint` per the repo's `apps/*` convention.
