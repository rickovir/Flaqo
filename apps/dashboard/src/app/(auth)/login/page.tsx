"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { ApiError, login } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      await login(email, password);
      router.push("/projects");
      router.refresh();
    } catch (err) {
      // The API returns a generic message for both an unknown address and
      // a wrong password, so the same text is shown for either case.
      setError(
        err instanceof ApiError
          ? err.message
          : "Sign in failed. Check your connection and try again.",
      );
      setSubmitting(false);
    }
  }

  return (
    <div className="login-page">
      <h1>Sign in</h1>
      <p className="subtitle">Manage your projects and feature flags.</p>

      <div className="card">
        {error ? <div className="error">{error}</div> : null}

        <form onSubmit={handleSubmit}>
          <div className="field">
            <label htmlFor="email">Email</label>
            <input
              id="email"
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>

          <div className="field">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          <div className="actions">
            <button className="primary" type="submit" disabled={submitting}>
              {submitting ? "Signing in..." : "Sign in"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
