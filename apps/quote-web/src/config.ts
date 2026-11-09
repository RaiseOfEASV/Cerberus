export function getApiUrl(): string {
  return (window as unknown as { __ENV__?: { API_URL?: string } }).__ENV__?.API_URL ?? "";
}
