"use client";

import { useState } from "react";
import {
  ApiError,
  createSdkKey,
  revokeSdkKey,
  type SdkKey,
} from "@/lib/api";

interface Props {
  projectId: string;
  environmentId: string;
  environmentName: string;
  keys: SdkKey[];
  onChanged: () => Promise<void> | void;
}

export function SdkKeyPanel({
  projectId,
  environmentId,
  environmentName,
  keys,
  onChanged,
}: Props) {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [keyType, setKeyType] = useState<"client" | "server">("client");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | undefined>();

  /**
   * The raw key, shown once.
   *
   * It is not retrievable afterwards, so the panel keeps it in memory
   * until the administrator dismisses it.
   */
  const [revealedKey, setRevealedKey] = useState<string | undefined>();

  async function handleCreate(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      const result = await createSdkKey(projectId, environmentId, name, keyType);
      setRevealedKey(result.key);
      setName("");
      setCreating(false);
      await onChanged();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Could not create the SDK key.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  async function handleRevoke(key: SdkKey) {
    const confirmed = window.confirm(
      `Revoke "${key.name}"? Applications using this key stop receiving configuration immediately.`,
    );
    if (!confirmed) {
      return;
    }

    setError(undefined);
    try {
      await revokeSdkKey(projectId, key.id);
      await onChanged();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Could not revoke the SDK key.",
      );
    }
  }

  const active = keys.filter((k) => !k.revoked_at);
  const revoked = keys.filter((k) => k.revoked_at);

  return (
    <>
      <h2>SDK keys</h2>
      <p className="subtitle">
        Keys are scoped to {environmentName}. A key can only read this one
        environment.
      </p>

      {error ? <div className="error">{error}</div> : null}

      {revealedKey ? (
        <div className="card" style={{ marginBottom: 16 }}>
          <h3 style={{ marginTop: 0 }}>Copy your key now</h3>
          <div className="key-reveal">{revealedKey}</div>
          <p className="muted" style={{ marginTop: 0 }}>
            This key is shown only once. It cannot be retrieved again, so
            store it somewhere safe now.
          </p>
          <div className="actions">
            <button onClick={() => setRevealedKey(undefined)}>
              I have saved it
            </button>
          </div>
        </div>
      ) : null}

      {creating ? (
        <div className="card" style={{ marginBottom: 16 }}>
          <h3 style={{ marginTop: 0 }}>New SDK key</h3>
          <form onSubmit={handleCreate}>
            <div className="field">
              <label htmlFor="key-name">Name</label>
              <input
                id="key-name"
                type="text"
                required
                maxLength={100}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="web-frontend"
              />
            </div>
            <div className="field">
              <label htmlFor="key-type">Type</label>
              <select
                id="key-type"
                value={keyType}
                onChange={(e) => setKeyType(e.target.value as "client" | "server")}
              >
                <option value="client">
                  Client - for browser and mobile applications
                </option>
                <option value="server">
                  Server - for backend applications, keep it secret
                </option>
              </select>
              <div className="hint">
                {keyType === "client"
                  ? "Client keys are visible to anyone using your application. They are read-only and scoped to this environment."
                  : "Server keys are confidential. Never include one in a browser bundle."}
              </div>
            </div>
            <div className="actions">
              <button className="primary" type="submit" disabled={submitting}>
                {submitting ? "Creating..." : "Create key"}
              </button>
              <button type="button" onClick={() => setCreating(false)}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      ) : null}

      <div className="card">
        <div className="row between" style={{ marginBottom: 12 }}>
          <strong>Keys</strong>
          <button className="small" onClick={() => setCreating((v) => !v)}>
            {creating ? "Cancel" : "New key"}
          </button>
        </div>

        {keys.length === 0 ? (
          <div className="empty">
            No SDK keys for {environmentName} yet.
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Key</th>
                <th>Status</th>
                <th style={{ width: 90 }} />
              </tr>
            </thead>
            <tbody>
              {[...active, ...revoked].map((key) => (
                <tr key={key.id}>
                  <td>{key.name}</td>
                  <td>
                    <span className="badge">{key.key_type}</span>
                  </td>
                  <td>
                    <code>{key.key_prefix}...</code>
                  </td>
                  <td>
                    {key.revoked_at ? (
                      <span className="badge revoked">revoked</span>
                    ) : (
                      <span className="badge on">active</span>
                    )}
                  </td>
                  <td>
                    <div className="row" style={{ justifyContent: "flex-end" }}>
                      {!key.revoked_at ? (
                        <button
                          className="small danger"
                          onClick={() => void handleRevoke(key)}
                        >
                          Revoke
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
