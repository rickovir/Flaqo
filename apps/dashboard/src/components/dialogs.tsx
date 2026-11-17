"use client";

import { useEffect, useState } from "react";
import { Dialog } from "./ui";

/** Creates a project. */
export function CreateProjectDialog({
  open,
  onClose,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  onCreate: (name: string, slug: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  // Reset the form each time the dialog opens.
  useEffect(() => {
    if (open) {
      setName("");
      setSlug("");
      setSlugEdited(false);
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  // The slug follows the name until it is edited by hand.
  function handleNameChange(value: string) {
    setName(value);
    if (!slugEdited) {
      setSlug(
        value
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, "-")
          .replace(/^-+|-+$/g, ""),
      );
    }
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      await onCreate(name, slug);
      onClose();
    } catch (err) {
      // The input is preserved so the administrator can correct it.
      setError(err instanceof Error ? err.message : "Could not create the project.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New project"
      description="A project holds your environments and feature flags."
      labelledBy="create-project-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            form="create-project-form"
            className="btn btn-primary"
            disabled={submitting}
          >
            {submitting ? "Creating..." : "Create project"}
          </button>
        </>
      }
    >
      <form id="create-project-form" onSubmit={handleSubmit}>
        {error ? (
          <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
            <div className="error-state-message">{error}</div>
          </div>
        ) : null}

        <div className="field">
          <label htmlFor="dialog-project-name">Name</label>
          <input
            id="dialog-project-name"
            type="text"
            required
            maxLength={200}
            value={name}
            onChange={(e) => handleNameChange(e.target.value)}
            placeholder="My Application"
          />
        </div>

        <div className="field">
          <label htmlFor="dialog-project-slug">Slug</label>
          <input
            id="dialog-project-slug"
            type="text"
            required
            maxLength={64}
            value={slug}
            onChange={(e) => {
              setSlug(e.target.value);
              setSlugEdited(true);
            }}
            placeholder="my-application"
          />
          <p className="field-hint">
            Lowercase letters, digits, and hyphens. The slug is a stable
            identifier and cannot be changed later.
          </p>
        </div>
      </form>
    </Dialog>
  );
}

/** Creates a feature flag. */
export function CreateFlagDialog({
  open,
  onClose,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  onCreate: (key: string, description: string) => Promise<void>;
}) {
  const [key, setKey] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setKey("");
      setDescription("");
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      await onCreate(key, description);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the flag.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New feature flag"
      description="The flag starts OFF in every environment."
      labelledBy="create-flag-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            form="create-flag-form"
            className="btn btn-primary"
            disabled={submitting}
          >
            {submitting ? "Creating..." : "Create flag"}
          </button>
        </>
      }
    >
      <form id="create-flag-form" onSubmit={handleSubmit}>
        {error ? (
          <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
            <div className="error-state-message">{error}</div>
          </div>
        ) : null}

        <div className="field">
          <label htmlFor="dialog-flag-key">Key</label>
          <input
            id="dialog-flag-key"
            type="text"
            required
            maxLength={64}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="new_dashboard"
          />
          <p className="field-hint">
            Lowercase letters, digits, underscore, and hyphen. This key is what
            your application reads.
          </p>
        </div>

        <div className="field">
          <label htmlFor="dialog-flag-description">Description (optional)</label>
          <input
            id="dialog-flag-description"
            type="text"
            maxLength={500}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this flag controls"
          />
        </div>
      </form>
    </Dialog>
  );
}

/** Edits a feature flag's description. */
export function EditFlagDialog({
  open,
  onClose,
  flagKey,
  onSave,
}: {
  open: boolean;
  onClose: () => void;
  flagKey: string | undefined;
  onSave: (description: string) => Promise<void>;
}) {
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setDescription("");
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      await onSave(description);
      onClose();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not update the flag.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Edit feature flag"
      description={flagKey}
      labelledBy="edit-flag-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            form="edit-flag-form"
            className="btn btn-primary"
            disabled={submitting}
          >
            {submitting ? "Saving..." : "Save changes"}
          </button>
        </>
      }
    >
      <form id="edit-flag-form" onSubmit={handleSubmit}>
        {error ? (
          <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
            <div className="error-state-message">{error}</div>
          </div>
        ) : null}

        <div className="field">
          <label htmlFor="dialog-edit-description">Description</label>
          <textarea
            id="dialog-edit-description"
            rows={3}
            maxLength={500}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this flag controls"
          />
          <p className="field-hint">
            The description is not part of the configuration your application
            receives, so changing it does not increment the version. Leaving it
            empty clears the description.
          </p>
        </div>
      </form>
    </Dialog>
  );
}

/**
 * Confirms a destructive action.
 *
 * The impact is stated plainly, and Production changes are called out
 * separately because they reach live traffic.
 */
export function DeleteConfirmationDialog({
  open,
  onClose,
  onConfirm,
  title,
  subject,
  impact,
  isProduction,
  confirmLabel = "Delete",
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
  title: string;
  subject: string;
  impact: string;
  isProduction?: boolean;
  confirmLabel?: string;
}) {
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  async function handleConfirm() {
    setError(undefined);
    setSubmitting(true);

    try {
      await onConfirm();
      onClose();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "The action could not be completed.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      labelledBy="delete-confirm-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-danger"
            onClick={() => void handleConfirm()}
            disabled={submitting}
          >
            {submitting ? "Working..." : confirmLabel}
          </button>
        </>
      }
    >
      {error ? (
        <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
          <div className="error-state-message">{error}</div>
        </div>
      ) : null}

      <p className="confirm-message">
        {subject} cannot be restored once it is deleted.
      </p>

      <div
        className={`confirm-impact${isProduction ? " confirm-impact-warning" : ""}`}
      >
        {impact}
      </div>
    </Dialog>
  );
}

/**
 * Confirms a Production state change.
 *
 * The design system requires that a potentially disruptive Production
 * change is never applied silently.
 */
export function ProductionChangeDialog({
  open,
  onClose,
  onConfirm,
  flagKey,
  environmentName,
  enabling,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
  flagKey: string;
  environmentName: string;
  enabling: boolean;
}) {
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  async function handleConfirm() {
    setError(undefined);
    setSubmitting(true);

    try {
      await onConfirm();
      onClose();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "The change could not be applied.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={`Turn ${flagKey} ${enabling ? "ON" : "OFF"}?`}
      description={`This affects the ${environmentName} environment.`}
      labelledBy="production-change-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => void handleConfirm()}
            disabled={submitting}
          >
            {submitting ? "Applying..." : `Yes, turn ${enabling ? "ON" : "OFF"}`}
          </button>
        </>
      }
    >
      {error ? (
        <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
          <div className="error-state-message">{error}</div>
        </div>
      ) : null}

      <div className="confirm-impact confirm-impact-warning">
        {enabling ? (
          <>
            <strong>This reaches live traffic.</strong> Applications connected
            to {environmentName} will pick up the change immediately, and any
            code path behind this flag will run.
          </>
        ) : (
          <>
            <strong>This reaches live traffic.</strong> Applications connected
            to {environmentName} will pick up the change immediately, and the
            code path behind this flag will stop running.
          </>
        )}
      </div>
    </Dialog>
  );
}

/** Generates an SDK key. */
export function GenerateSDKKeyDialog({
  open,
  onClose,
  environmentName,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  environmentName: string;
  onCreate: (
    name: string,
    keyType: "client" | "server",
  ) => Promise<{ key: string }>;
}) {
  const [name, setName] = useState("");
  const [keyType, setKeyType] = useState<"client" | "server">("client");
  const [error, setError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setName("");
      setKeyType("client");
      setError(undefined);
      setSubmitting(false);
    }
  }, [open]);

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setSubmitting(true);

    try {
      await onCreate(name, keyType);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the key.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Generate SDK key"
      description={`The key is scoped to the ${environmentName} environment.`}
      labelledBy="generate-key-title"
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            form="generate-key-form"
            className="btn btn-primary"
            disabled={submitting}
          >
            {submitting ? "Generating..." : "Generate key"}
          </button>
        </>
      }
    >
      <form id="generate-key-form" onSubmit={handleSubmit}>
        {error ? (
          <div className="error-state" role="alert" style={{ marginBottom: "var(--space-4)" }}>
            <div className="error-state-message">{error}</div>
          </div>
        ) : null}

        <div className="field">
          <label htmlFor="dialog-key-name">Name</label>
          <input
            id="dialog-key-name"
            type="text"
            required
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="web-frontend"
          />
          <p className="field-hint">A label so you can recognise the key later.</p>
        </div>

        <div className="field">
          <label htmlFor="dialog-key-type">Type</label>
          <select
            id="dialog-key-type"
            value={keyType}
            onChange={(e) => setKeyType(e.target.value as "client" | "server")}
          >
            <option value="client">Client - for browser and mobile applications</option>
            <option value="server">Server - for backend applications, keep it secret</option>
          </select>
          <p className="field-hint">
            {keyType === "client"
              ? "Client keys are visible to anyone using your application. They are read-only and scoped to this environment."
              : "Server keys are confidential. Never include one in a browser bundle."}
          </p>
        </div>
      </form>
    </Dialog>
  );
}
