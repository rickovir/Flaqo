"use client";

import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { EnvironmentSelector } from "@/components/EnvironmentSelector";
import { FeatureFlagToggle } from "@/components/FeatureFlagTable";
import {
  CreateFlagDialog,
  DeleteConfirmationDialog,
  EditFlagDialog,
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
  setFlagState,
  updateFlagDescription,
  type Environment,
  type FlagValue,
} from "@/lib/api";

const PANEL_ID = "flags-panel";

export default function ProjectDetailPage() {
  const params = useParams<{ projectId: string }>();
  const projectId = params.projectId;

  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [selectedEnvId, setSelectedEnvId] = useState<string | undefined>();
  const [flags, setFlags] = useState<FlagValue[]>([]);
  const [loadingEnvironments, setLoadingEnvironments] = useState(true);
  const [loadingFlags, setLoadingFlags] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<FlagValue | undefined>();
  const [deleting, setDeleting] = useState<FlagValue | undefined>();

  // A Production change is held until the administrator confirms it.
  const [pendingProduction, setPendingProduction] = useState<{
    flag: FlagValue;
    enabled: boolean;
  } | undefined>();

  const { notify } = useToast();

  const loadEnvironments = useCallback(async () => {
    try {
      const result = await listEnvironments(projectId);
      setEnvironments(result);
      setSelectedEnvId((current) => current ?? result[0]?.id);
      setError(undefined);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(
          err instanceof ApiError
            ? err.message
            : "Could not load environments.",
        );
      }
    } finally {
      setLoadingEnvironments(false);
    }
  }, [projectId]);

  const loadFlags = useCallback(async () => {
    if (!selectedEnvId) {
      return;
    }
    setLoadingFlags(true);
    try {
      const result = await listFlags(projectId, selectedEnvId);
      setFlags(result);
      setError(undefined);
    } catch (err) {
      if (!(err instanceof ApiError && err.isUnauthorized)) {
        setError(err instanceof ApiError ? err.message : "Could not load flags.");
      }
    } finally {
      setLoadingFlags(false);
    }
  }, [projectId, selectedEnvId]);

  useEffect(() => {
    void loadEnvironments();
  }, [loadEnvironments]);

  useEffect(() => {
    if (selectedEnvId) {
      void loadFlags();
    }
  }, [selectedEnvId, loadFlags]);

  const selectedEnv = environments.find((env) => env.id === selectedEnvId);
  const isProduction = selectedEnv?.key === "production";

  async function handleCreate(key: string, description: string) {
    await createFlag(projectId, key, description);
    await loadFlags();
    notify(`Created "${key}". It is OFF in every environment.`);
  }

  async function handleToggle(flag: FlagValue, enabled: boolean): Promise<void> {
    if (!selectedEnvId) {
      return;
    }

    // A Production change is confirmed before it is applied.
    if (isProduction) {
      setPendingProduction({ flag, enabled });
      return;
    }

    const result = await setFlagState(flag.id, selectedEnvId, enabled);
    await loadFlags();
    notify(
      result.changed
        ? `"${flag.key}" is now ${enabled ? "ON" : "OFF"}.`
        : `"${flag.key}" was already ${enabled ? "ON" : "OFF"}.`,
    );
  }

  async function handleConfirmProduction() {
    if (!pendingProduction || !selectedEnvId) {
      return;
    }
    const { flag, enabled } = pendingProduction;
    const result = await setFlagState(flag.id, selectedEnvId, enabled);
    setPendingProduction(undefined);
    await loadFlags();
    notify(
      result.changed
        ? `"${flag.key}" is now ${enabled ? "ON" : "OFF"} in production.`
        : `"${flag.key}" was already ${enabled ? "ON" : "OFF"}.`,
    );
  }

  async function handleEditDescription(description: string) {
    if (!editing) {
      return;
    }
    await updateFlagDescription(editing.id, description);
    await loadFlags();
    notify(`Updated "${editing.key}".`);
  }

  async function handleDelete(flag: FlagValue) {
    await deleteFlag(flag.id);
    await loadFlags();
    notify(`Deleted "${flag.key}".`);
  }

  if (loadingEnvironments) {
    return <LoadingState label="Loading project" />;
  }

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Projects", href: "/projects" },
          { label: "Feature Flags" },
        ]}
      />

      <PageHeader
        title="Feature flags"
        description="Each flag is independent per environment."
        actions={
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => setCreating(true)}
          >
            New flag
          </button>
        }
      />

      {error ? (
        <div style={{ marginBottom: "var(--space-4)" }}>
          <ErrorState message={error} onRetry={() => void loadFlags()} />
        </div>
      ) : null}

      <EnvironmentSelector
        environments={environments}
        selectedId={selectedEnvId}
        onSelect={setSelectedEnvId}
        panelId={PANEL_ID}
      />

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
        aria-labelledby={selectedEnvId ? `tab-${selectedEnvId}` : undefined}
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
                          className="btn btn-sm"
                          onClick={() => setEditing(flag)}
                        >
                          Edit
                        </button>
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

      <EditFlagDialog
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        flagKey={editing?.key}
        onSave={handleEditDescription}
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
