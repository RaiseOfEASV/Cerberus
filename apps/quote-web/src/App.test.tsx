import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, afterEach } from "vitest";
import App from "./App";
import * as api from "./api";

afterEach(() => vi.restoreAllMocks());

describe("App", () => {
  it("shows the quote once it loads", async () => {
    vi.spyOn(api, "fetchQuote").mockResolvedValue("hello world");
    render(<App />);
    await waitFor(() => expect(screen.getByText("hello world")).toBeInTheDocument());
  });

  it("shows an error message when the fetch fails", async () => {
    vi.spyOn(api, "fetchQuote").mockRejectedValue(new Error("boom"));
    render(<App />);
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
  });
});
