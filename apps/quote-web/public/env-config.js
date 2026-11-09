// Fallback for local `npm run dev` only. In the container this file is
// generated at start-up from the API_URL env var (docker/docker-entrypoint.sh),
// so the same image can be promoted dev -> staging -> prod unchanged.
window.__ENV__ = { API_URL: "http://localhost:8080" };
