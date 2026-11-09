import { describe, it, expect, vi, afterEach } from "vitest";
import { fetchQuote } from "./api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("fetchQuote", () => {
  it("returns the quote on a successful response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ quote: "hello" }),
    }));
    await expect(fetchQuote()).resolves.toBe("hello");
  });

  it("throws when the response is not ok", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 500 }));
    await expect(fetchQuote()).rejects.toThrow();
  });
});
