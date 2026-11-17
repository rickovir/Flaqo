"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  createFlag,
  deleteFlag,
  listEnvironments,
  listFlags,
  listSdkKeys,
  setFlagState,
  type Environment,
  type FlagValue,
  type SdkKey,
} from "@/lib/api";
import { useSession } from "@/lib/useSession";
import { SdkKeyPanel } from "./SdkKeyPanel";

export default function ProjectDetailPage() {
  const params = useParams<{ projectId: string }>();
  const projectId = params.projectId;
  const router = useRouter();

  const { user, loading: sessionLoading } = useSession();

  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [selectedEnvId, setSelectedEnvId] = useState<string | undefined>();
  const [flags, setFlags] = useState<FlagValue[]>([]);
  const [sdkKeys, setSdkKeys] = useState<SdkKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | undefined>();

  const [creatingFlag, setCreatingFlag] = useState(false);
  const [flagKey, setFlagKey] = useState("");
  const [flagDescription, setFlagDescription] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const loadEnvironments = useCallback(async () => {
    try {
      const result = await listEnvironments(projectId);
      setEnvironments(result);
      setSelectedEnvId((current) => current ?? result[0]?.id);
      setError(undefined);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(
          err instanceof ApiError ? err.message : "Could not load environments.",
        );
      }
    } finally {
      // The page is ready once the environments are known. The flags and
      // keys load for the selected environment.
      setLoading(false);
    }
  }, [projectId]);

  const loadFlags = useCallback(async () => {
    if (!selectedEnvId) {
      return;
    }
    try {
      const result = await listFlags(projectId, selectedEnvId);
      setFlags(result);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(err instanceof ApiError ? err.message : "Could not load flags.");
      }
    }
  }, [projectId, selectedEnvId]);

  const loadKeys = useCallback(async () => {
    if (!selectedEnvId) {
      return;
    }
    try {
      const result = await listSdkKeys(projectId, selectedEnvId);
      setSdkKeys(result);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(err instanceof ApiError ? err.message : "Could not load SDK keys.");
      }
    }
  }, [projectId, selectedEnvId]);

  useEffect(() => {
    if (!sessionLoading) {
      void loadEnvironments();
    }
  }, [sessionLoading, loadEnvironments]);

  // Without a session the page is not usable, so it is redirected to the
  // sign in page.
  useEffect(() => {
    if (!sessionLoading && !user) {
      router.replace("/login");
    }
  }, [sessionLoading, user, router]);

  useEffect(() => {
    if (!sessionLoading && selectedEnvId) {
      void loadFlags();
      void loadKeys();
    }
  }, [sessionLoading, selectedEnvId, loadFlags, loadKeys]);

  async function handleCreateFlag(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setNotice(undefined);
    setSubmitting(true);

    try {
      await createFlag(projectId, flagKey, flagDescription);
      setFlagKey("");
      setFlagDescription("");
      setCreatingFlag(false);
      setNotice(`Created "${flagKey}". It is OFF in every environment.`);
      await loadFlags();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create the flag.");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleToggle(flag: FlagValue, enabled: boolean) {
    if (!selectedEnvId) {
      return;
    }
    setError(undefined);
    setNotice(undefined);

    try {
      const result = await setFlagState(flag.id, selectedEnvId, enabled);
      await loadFlags();
      if (!result.changed) {
        // The write was a no-op, so the version did not move.
        setNotice(`"${flag.key}" was already ${enabled ? "ON" : "OFF"}.`);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update the flag.");
      // Reload so the toggle reflects what the server actually holds.
      await loadFlags();
    }
  }

  async function handleDeleteFlag(flag: FlagValue) {
    const confirmed = window.confirm(`Delete "${flag.key}"?`);
    if (!confirmed) {
      return;
    }

    setError(undefined);
    try {
      await deleteFlag(flag.id);
      await loadFlags();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete the flag.");
    }
  }

  const selectedEnv = environments.find((e) => e.id === selectedEnvId);

  if (sessionLoading || loading) {
    return <div className="spinner">Loading...</div>;
  }

  return (
    <>
      <div className="row between" style={{ marginBottom: 20 }}>
        <div>
          <h1>Feature flags</h1>
          <p className="subtitle" style={{ marginBottom: 0 }}>
            Each flag is independent per environment.
          </p>
        </div>
        <button onClick={() => setCreatingFlag((v) => !v)}>
          {creatingFlag ? "Cancel" : "New flag"}
        </button>
      </div>

      {error ? <div className="error">{error}</div> : null}
      {notice ? <div className="success">{notice}</div> : null}

      <div className="env-tabs" role="tablist" aria-label="Environments">
        {environments.map((env) => (
          <button
            key={env.id}
            role="tab"
            aria-selected={env.id === selectedEnvId}
            onClick={() => setSelectedEnvId(env.id)}
          >
            {env.name}
            <span className="muted" style={{ marginLeft: 6, fontSize: 12 }}>
              v{env.config_version}
            </span>
          </button>
        ))}
      </div>

      {creatingFlag ? (
        <div className="card" style={{ marginBottom: 20 }}>
          <h3 style={{ marginTop: 0 }}>New flag</h3>
          <form onSubmit={handleCreateFlag}>
            <div className="field">
              <label htmlFor="flag-key">Key</label>
              <input
                id="flag-key"
                type="text"
                required
                maxLength={64}
                pattern="[a-z0-9][a-z0-9_\-]*"
                value={flagKey}
                onChange={(e) => setFlagKey(e.target.value)}
                placeholder="new_dashboard"
              />
              <div className="hint">
                Lowercase letters, digits, underscore, and hyphen. This key is
                what your application reads.
              </div>
            </div>
            <div className="field">
              <label htmlFor="flag-description">Description (optional)</label>
              <input
                id="flag-description"
                type="text"
                maxLength={500}
                value={flagDescription}
                onChange={(e) => setFlagDescription(e.target.value)}
              />
            </div>
            <div className="actions">
              <button className="primary" type="submit" disabled={submitting}>
                {submitting ? "Creating..." : "Create flag"}
              </button>
            </div>
          </form>
        </div>
      ) : null}

      <div className="card">
        {flags.length === 0 ? (
          <div className="empty">
            No flags in {selectedEnv?.name ?? "this environment"} yet.
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th style={{ width: 70 }}>State</th>
                <th>Key</th>
                <th style={{ width: 90 }} />
              </tr>
            </thead>
            <tbody>
              {flags.map((flag) => (
                <tr key={flag.id}>
                  <td>
                    <label className="toggle">
                      <input
                        type="checkbox"
                        checked={flag.enabled}
                        onChange={(e) => void handleToggle(flag, e.target.checked)}
                        aria-label={`${flag.key} in ${selectedEnv?.name ?? "environment"}`}
                      />
                      <span className="track" />
                      <span className="thumb" />
                    </label>
                  </td>
                  <td>
                    <code>{flag.key}</code>{" "}
                    <span className={`badge ${flag.enabled ? "on" : "off"}`}>
                      {flag.enabled ? "ON" : "OFF"}
                    </span>
                  </td>
                  <td>
                    <div className="row" style={{ justifyContent: "flex-end" }}>
                      <button
                        className="small danger"
                        onClick={() => void handleDeleteFlag(flag)}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {selectedEnvId ? (
        <SdkKeyPanel
          projectId={projectId}
          environmentId={selectedEnvId}
          environmentName={selectedEnv?.name ?? "environment"}
          keys={sdkKeys}
          onChanged={loadKeys}
        />
      ) : null}
    </>
  );
}
