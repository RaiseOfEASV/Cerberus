import { useEffect, useState } from "react";
import { fetchQuote } from "./api";
import "./App.css";

export default function App() {
  const [quote, setQuote] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchQuote()
      .then((q) => {
        if (!cancelled) setQuote(q);
      })
      .catch(() => {
        if (!cancelled) setError("Couldn't load a quote right now. Please try again later.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <main className="page">
      <h1>Cerberoast</h1>
      {error !== null ? (
        <p role="alert" className="error">
          {error}
        </p>
      ) : quote !== null ? (
        <blockquote className="quote">{quote}</blockquote>
      ) : (
        <p className="loading" aria-busy="true">
          Loading a quote…
        </p>
      )}
    </main>
  );
}
