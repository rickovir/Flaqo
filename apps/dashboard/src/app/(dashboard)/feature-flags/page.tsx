"use client";

import { useEffect, useState } from "react";
import { EnvironmentSelector } from "@/components/EnvironmentSelector";
import { FeatureFlagToggle } from "@/components/FeatureFlagTable";
import {
  CreateFlagDialog,
  DeleteConfirmationDialog,
  ProductionChangeDialog,
} from "@/components/dialogs";
import {
  Breadcrumbs,
  CopyButton,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
} from "@/components/primitives";
import { useToast } from "@/components/ToastProvider";
import {
  ApiError,
  createFlag,
  deleteFlag,
  listEnvironments,
  listFlags,
  listProjects,
  setFlagState,
  type Environment,
  type FlagValue,
  type Project,
} from "@/lib/api";

const PANEL_ID = "flags-panel";

/**
 * The feature flag list across every project.
 *
 * The design system treats Feature Flags as its own destination, so it
 * has its own page rather than living under a project.
 */
export default function FeatureFlagsPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<string | undefined>();
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [envId, setEnvId] = useState<string | undefined>();
  const [flags, setFlags] = useState<FlagValue[]>([]);
  const [loadingProjects, setLoadingProjects] = useState(true);
  const [loadingFlags, setLoadingFlags] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<FlagValue | undefined>();
  const [pendingProduction, setPendingProduction] = useState<{
    flag: FlagValue;
    enabled: boolean;
  } | undefined>();

  const { notify } = useToast();

  useEffect(() => {
    listProjects()
      .then((result) => {
        setProjects(result);
        setProjectId((current) => current ?? result[0]?.id);
      })
      .catch((err) => {
        if (!(err instanceof ApiError && err.isUnauthorized)) {
          setError(
            err instanceof ApiError ? err.message : "Could not load projects.",
          );
        }
      })
      .finally(() => setLoadingProjects(false));
  }, []);

  useEffect(() => {
    if (!projectId) {
      return;
    }
    listEnvironments(projectId)
      .then((result) => {
        setEnvironments(result);
        setEnvId((current) => current ?? result[0]?.id);
      })
      .catch((err) => {
        if (!(err instanceof ApiError && err.isUnauthorized)) {
          setError(
            err instanceof ApiError ? err.message : "Could not load environments.",
          );
        }
      });
  }, [projectId]);

  useEffect(() => {
    if (!projectId || !envId) {
      return;
    }
    setLoadingFlags(true);
    listFlags(projectId, envId)
      .then((result) => {
        setFlags(result);
        setError(undefined);
      })
      .catch((err) => {
        if (!(err instanceof ApiError && err.isUnauthorized)) {
          setError(err instanceof ApiError ? err.message : "Could not load flags.");
        }
      })
      .finally(() => setLoadingFlags(false));
  }, [projectId, envId]);

  const selectedEnv = environments.find((env) => env.id === envId);
  const isProduction = selectedEnv?.key === "production";

  async function handleCreate(key: string, description: string) {
    if (!projectId) {
      return;
    }
    await createFlag(projectId, key, description);
    setFlags(await listFlags(projectId, envId!));
    notify(`Created "${key}". It is OFF in every environment.`);
  }

  async function handleToggle(flag: FlagValue, enabled: boolean): Promise<void> {
    if (!envId) {
      return;
    }
    if (isProduction) {
      setPendingProduction({ flag, enabled });
      return;
    }
    const result = await setFlagState(flag.id, envId, enabled);
    setFlags(await listFlags(projectId!, envId));
    notify(
      result.changed
        ? `"${flag.key}" is now ${enabled ? "ON" : "OFF"}.`
        : `"${flag.key}" was already ${enabled ? "ON" : "OFF"}.`,
    );
  }

  async function handleConfirmProduction() {
    if (!pendingProduction || !envId) {
      return;
    }
    const { flag, enabled } = pendingProduction;
    const result = await setFlagState(flag.id, envId, enabled);
    setPendingProduction(undefined);
    setFlags(await listFlags(projectId!, envId));
    notify(
      result.changed
        ? `"${flag.key}" is now ${enabled ? "ON" : "OFF"} in production.`
        : `"${flag.key}" was already ${enabled ? "ON" : "OFF"}.`,
    );
  }

  async function handleDelete(flag: FlagValue) {
    await deleteFlag(flag.id);
    setFlags(await listFlags(projectId!, envId!));
    notify(`Deleted "${flag.key}".`);
  }

  if (loadingProjects) {
    return <LoadingState label="Loading feature flags" />;
  }

  return (
    <>
      <Breadcrumbs items={[{ label: "Feature Flags" }]} />

      <PageHeader
        title="Feature flags"
        description="Boolean flags, independent per environment."
        actions={
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => setCreating(true)}
            disabled={!projectId}
          >
            New flag
          </button>
        }
      />

      {error ? (
        <div style={{ marginBottom: "var(--space-4)" }}>
          <ErrorState
            message={error}
            onRetry={() => window.location.reload()}
          />
        </div>
      ) : null}

      {projects.length > 1 ? (
        <div className="field" style={{ maxWidth: 320 }}>
          <label htmlFor="flags-project">Project</label>
          <select
            id="flags-project"
            value={projectId ?? ""}
            onChange={(e) => {
              setProjectId(e.target.value);
              setEnvId(undefined);
              setFlags([]);
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
          panelId={PANEL_ID}
        />
      ) : null}

      {isProduction ? (
        <div className="prod-notice" role="note">
          <span className="prod-notice-icon" aria-hidden="true">
            !
          </span>
          <span>
            <strong>You are editing Production.</strong> Changes here reach
            live traffic as soon as connected applications pick them up. You
            will be asked to confirm each state change.
          </span>
        </div>
      ) : null}

      <div
        className="card"
        id={PANEL_ID}
        role="tabpanel"
        aria-labelledby={envId ? `tab-${envId}` : undefined}
      >
        {loadingFlags ? (
          <LoadingState label="Loading flags" rows={3} columns={3} />
        ) : flags.length === 0 ? (
          <EmptyState
            title={`No flags in ${selectedEnv?.name ?? "this environment"}`}
            description="Create a flag to control a feature for your applications."
            action={
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => setCreating(true)}
              >
                New flag
              </button>
            }
          />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col" style={{ width: 140 }}>
                    State
                  </th>
                  <th scope="col">Key</th>
                  <th scope="col" style={{ width: 120 }}>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {flags.map((flag) => (
                  <tr key={flag.id}>
                    <td>
                      <FeatureFlagToggle
                        flag={flag}
                        environmentName={selectedEnv?.name ?? "environment"}
                        isProduction={isProduction}
                        onChange={handleToggle}
                        onError={(message) => notify(message, "error")}
                      />
                    </td>
                    <td>
                      <span className="row" style={{ gap: "var(--space-1)" }}>
                        <code>{flag.key}</code>
                        <CopyButton value={flag.key} label="Copy flag key" />
                      </span>
                    </td>
                    <td>
                      <div className="cell-actions">
                        <button
                          type="button"
                          className="btn btn-sm btn-danger"
                          onClick={() => setDeleting(flag)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <CreateFlagDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreate={handleCreate}
      />

      <DeleteConfirmationDialog
        open={deleting !== undefined}
        onClose={() => setDeleting(undefined)}
        onConfirm={async () => {
          if (deleting) {
            await handleDelete(deleting);
          }
        }}
        title="Delete feature flag?"
        subject={`"${deleting?.key ?? ""}"`}
        impact="The flag is removed from every environment and applications stop seeing it. This cannot be undone."
      />

      <ProductionChangeDialog
        open={pendingProduction !== undefined}
        onClose={() => setPendingProduction(undefined)}
        onConfirm={handleConfirmProduction}
        flagKey={pendingProduction?.flag.key ?? ""}
        environmentName={selectedEnv?.name ?? "production"}
        enabling={pendingProduction?.enabled ?? false}
      />
    </>
  );
}
