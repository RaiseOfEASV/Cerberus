import { getApiUrl } from "./config";

export interface QuoteResponse {
  quote: string;
}

export async function fetchQuote(): Promise<string> {
  const res = await fetch(`${getApiUrl()}/quote`);
  if (!res.ok) {
    throw new Error(`request failed: ${res.status}`);
  }
  const data = (await res.json()) as QuoteResponse;
  return data.quote;
}
