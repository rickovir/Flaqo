"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  CreateProjectDialog,
  DeleteConfirmationDialog,
} from "@/components/dialogs";
import { CopyButton, EmptyState, ErrorState, LoadingState } from "@/components/primitives";
import { useToast } from "@/components/ToastProvider";
import {
  ApiError,
  createProject,
  deleteProject,
  listProjects,
  type Project,
} from "@/lib/api";

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<Project | undefined>();

  const { notify } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await listProjects();
      setProjects(result);
      setError(undefined);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        return;
      }
      setError(
        err instanceof ApiError ? err.message : "Could not load projects.",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleCreate(name: string, slug: string) {
    const result = await createProject(name, slug);
    await load();
    notify(`Created "${result.project.name}".`);
  }

  async function handleDelete(project: Project) {
    await deleteProject(project.id);
    await load();
    notify(`Deleted "${project.name}".`);
  }

  return (
    <>
      <header className="page-header">
        <div className="page-title-group">
          <h1>Projects</h1>
          <p className="page-description">
            Each project holds its own environments and feature flags.
          </p>
        </div>
        <div className="page-actions">
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => setCreating(true)}
          >
            New project
          </button>
        </div>
      </header>

      {error ? (
        <div style={{ marginBottom: "var(--space-4)" }}>
          <ErrorState message={error} onRetry={() => void load()} />
        </div>
      ) : null}

      <div className="card">
        {loading ? (
          <LoadingState
            label="Loading projects"
            rows={3}
            columns={3}
          />
        ) : projects.length === 0 ? (
          <EmptyState
            title="No projects yet"
            description="Create a project to get its environments and feature flags."
            action={
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => setCreating(true)}
              >
                Create your first project
              </button>
            }
          />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">Name</th>
                  <th scope="col">Slug</th>
                  <th scope="col" style={{ width: 160 }}>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {projects.map((project) => (
                  <tr key={project.id}>
                    <td>
                      <Link
                        href={`/projects/${project.id}`}
                        style={{ fontWeight: 500 }}
                      >
                        {project.name}
                      </Link>
                    </td>
                    <td>
                      <span className="row" style={{ gap: "var(--space-1)" }}>
                        <code>{project.slug}</code>
                        <CopyButton value={project.slug} label="Copy slug" />
                      </span>
                    </td>
                    <td>
                      <div className="cell-actions">
                        <Link
                          href={`/projects/${project.id}`}
                          className="btn btn-sm"
                          aria-label={`Manage ${project.name}`}
                        >
                          Manage
                        </Link>
                        <button
                          type="button"
                          className="btn btn-sm btn-danger"
                          onClick={() => setDeleting(project)}
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

      <CreateProjectDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreate={handleCreate}
      />

      <DeleteProjectDialog
        project={deleting}
        onClose={() => setDeleting(undefined)}
        onConfirm={handleDelete}
      />
    </>
  );
}

/** Confirms a project deletion. */
function DeleteProjectDialog({
  project,
  onClose,
  onConfirm,
}: {
  project: Project | undefined;
  onClose: () => void;
  onConfirm: (project: Project) => Promise<void>;
}) {
  return (
    <DeleteConfirmationDialog
      open={project !== undefined}
      onClose={onClose}
      onConfirm={() => (project ? onConfirm(project) : Promise.resolve())}
      title="Delete project?"
      subject={`"${project?.name ?? ""}"`}
      impact="Its environments, feature flags, flag states, and SDK keys are deleted with it. Applications using its SDK keys stop receiving configuration."
    />
  );
}
