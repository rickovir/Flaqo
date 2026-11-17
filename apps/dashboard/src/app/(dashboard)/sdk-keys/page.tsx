"use client";

import { useCallback, useEffect, useState } from "react";
import { EnvironmentSelector } from "@/components/EnvironmentSelector";
import {
  DeleteConfirmationDialog,
  GenerateSDKKeyDialog,
} from "@/components/dialogs";
import {
  CopyButton,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
  StatusBadge,
} from "@/components/primitives";
import { useToast } from "@/components/ToastProvider";
import {
  ApiError,
  createSdkKey,
  listEnvironments,
  listProjects,
  listSdkKeys,
  revokeSdkKey,
  type Environment,
  type Project,
  type SdkKey,
} from "@/lib/api";

/**
 * SDK key management.
 *
 * The design system keeps this separate from feature flags, so it has
 * its own page with its own environment context.
 */
export default function SdkKeysPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<string | undefined>();
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [envId, setEnvId] = useState<string | undefined>();
  const [keys, setKeys] = useState<SdkKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();
  const [generating, setGenerating] = useState(false);
  const [revoking, setRevoking] = useState<SdkKey | undefined>();
  const [revealed, setRevealed] = useState<string | undefined>();

  const { notify } = useToast();

  const loadProjects = useCallback(async () => {
    try {
      const result = await listProjects();
      setProjects(result);
      setProjectId((current) => current ?? result[0]?.id);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(err instanceof ApiError ? err.message : "Could not load projects.");
      }
    }
  }, []);

  const loadEnvironments = useCallback(async () => {
    if (!projectId) {
      return;
    }
    try {
      const result = await listEnvironments(projectId);
      setEnvironments(result);
      setEnvId((current) => current ?? result[0]?.id);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(
          err instanceof ApiError ? err.message : "Could not load environments.",
        );
      }
    }
  }, [projectId]);

  const loadKeys = useCallback(async () => {
    if (!projectId || !envId) {
      return;
    }
    try {
      const result = await listSdkKeys(projectId, envId);
      setKeys(result);
      setError(undefined);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(err instanceof ApiError ? err.message : "Could not load SDK keys.");
      }
    } finally {
      setLoading(false);
    }
  }, [projectId, envId]);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  useEffect(() => {
    if (projectId) {
      void loadEnvironments();
    }
  }, [projectId, loadEnvironments]);

  useEffect(() => {
    if (projectId && envId) {
      void loadKeys();
    }
  }, [projectId, envId, loadKeys]);

  const selectedEnv = environments.find((env) => env.id === envId);

  async function handleCreate(
    name: string,
    keyType: "client" | "server",
  ): Promise<{ key: string }> {
    if (!projectId || !envId) {
      return { key: "" };
    }
    const result = await createSdkKey(projectId, envId, name, keyType);
    // The raw key is shown once and cannot be retrieved again.
    setRevealed(result.key);
    await loadKeys();
    notify("SDK key generated. Copy it now.");
    return result;
  }

  async function handleRevoke(key: SdkKey) {
    if (!projectId) {
      return;
    }
    await revokeSdkKey(projectId, key.id);
    await loadKeys();
    notify(`Revoked "${key.name}".`);
  }

  return (
    <>
      <PageHeader
        title="SDK keys"
        description="Keys grant read-only access to one environment. Revoking a key takes effect immediately."
        actions={
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => setGenerating(true)}
            disabled={!envId}
          >
            Generate key
          </button>
        }
      />

      {error ? (
        <div style={{ marginBottom: "var(--space-4)" }}>
          <ErrorState message={error} onRetry={() => void loadKeys()} />
        </div>
      ) : null}

      {revealed ? (
        <div className="card" style={{ marginBottom: "var(--space-4)" }}>
          <div className="card-header">
            <h2 className="card-title">Copy your key now</h2>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => setRevealed(undefined)}
            >
              I have saved it
            </button>
          </div>
          <div className="card-body">
            <div className="key-reveal">
              <span className="key-reveal-value">{revealed}</span>
              <CopyButton value={revealed} label="Copy SDK key" />
            </div>
            <p className="field-hint" style={{ marginTop: 0 }}>
              This key is shown only once. It cannot be retrieved again, so
              store it somewhere safe now.
            </p>
          </div>
        </div>
      ) : null}

      {projects.length > 1 ? (
        <div className="field" style={{ maxWidth: 320 }}>
          <label htmlFor="sdk-project">Project</label>
          <select
            id="sdk-project"
            value={projectId ?? ""}
            onChange={(e) => {
              setProjectId(e.target.value);
              setEnvId(undefined);
              setKeys([]);
            }}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </div>
      ) : null}

      {environments.length > 0 ? (
        <EnvironmentSelector
          environments={environments}
          selectedId={envId}
          onSelect={setEnvId}
          panelId="sdk-keys-panel"
        />
      ) : null}

      <div
        className="card"
        id="sdk-keys-panel"
        role="tabpanel"
        aria-labelledby={envId ? `tab-${envId}` : undefined}
      >
        <div className="card-header">
          <h2 className="card-title">
            Keys for {selectedEnv?.name ?? "this environment"}
          </h2>
        </div>

        {loading ? (
          <LoadingState label="Loading SDK keys" rows={3} columns={4} />
        ) : keys.length === 0 ? (
          <EmptyState
            title={`No SDK keys for ${selectedEnv?.name ?? "this environment"}`}
            description="Generate a key so an application can read this environment's configuration."
            action={
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => setGenerating(true)}
              >
                Generate key
              </button>
            }
          />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">Name</th>
                  <th scope="col">Type</th>
                  <th scope="col">Key identifier</th>
                  <th scope="col">Status</th>
                  <th scope="col" style={{ width: 100 }}>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {keys.map((key) => (
                  <tr key={key.id}>
                    <td>{key.name}</td>
                    <td>
                      <StatusBadge
                        tone={key.key_type === "server" ? "warning" : "neutral"}
                      >
                        {key.key_type}
                      </StatusBadge>
                    </td>
                    <td>
                      <span className="row" style={{ gap: "var(--space-1)" }}>
                        <code>{key.key_prefix}...</code>
                        <CopyButton value={key.key_prefix} label="Copy key prefix" />
                      </span>
                    </td>
                    <td>
                      {key.revoked_at ? (
                        <StatusBadge tone="revoked">revoked</StatusBadge>
                      ) : (
                        <StatusBadge tone="on">active</StatusBadge>
                      )}
                    </td>
                    <td>
                      <div className="cell-actions">
                        {key.revoked_at ? null : (
                          <button
                            type="button"
                            className="btn btn-sm btn-danger"
                            onClick={() => setRevoking(key)}
                          >
                            Revoke
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <GenerateSDKKeyDialog
        open={generating}
        onClose={() => setGenerating(false)}
        environmentName={selectedEnv?.name ?? "this environment"}
        onCreate={handleCreate}
      />

      <DeleteConfirmationDialog
        open={revoking !== undefined}
        onClose={() => setRevoking(undefined)}
        onConfirm={async () => {
          if (revoking) {
            await handleRevoke(revoking);
          }
        }}
        title="Revoke SDK key?"
        subject={`"${revoking?.name ?? ""}"`}
        impact="Applications using this key stop receiving configuration immediately, and any stream it holds is closed. Revocation cannot be undone."
        confirmLabel="Revoke key"
      />
    </>
  );
}
