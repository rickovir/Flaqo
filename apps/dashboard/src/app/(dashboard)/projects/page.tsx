"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  createProject,
  deleteProject,
  listProjects,
  type Project,
} from "@/lib/api";
import { useSession } from "@/lib/useSession";

export default function ProjectsPage() {
  const { user, loading: sessionLoading, signOut } = useSession();
  const router = useRouter();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | undefined>();

  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    try {
      const result = await listProjects();
      setProjects(result);
      setError(undefined);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        // The session expired. The sign in page will handle it.
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
    if (!sessionLoading && user) {
      void load();
    }
  }, [sessionLoading, user, load]);

  // Without a session the page is not usable, so it is redirected to the
  // sign in page rather than showing an empty table.
  useEffect(() => {
    if (!sessionLoading && !user) {
      router.replace("/login");
    }
  }, [sessionLoading, user, router]);

  async function handleCreate(event: React.FormEvent) {
    event.preventDefault();
    setError(undefined);
    setNotice(undefined);
    setSubmitting(true);

    try {
      const result = await createProject(name, slug);
      setName("");
      setSlug("");
      setCreating(false);
      setNotice(
        `Created "${result.project.name}" with ${result.environments.length} environments.`,
      );
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create the project.");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(project: Project) {
    const confirmed = window.confirm(
      `Delete "${project.name}"? Its environments, flags, and SDK keys are removed too.`,
    );
    if (!confirmed) {
      return;
    }

    setError(undefined);
    try {
      await deleteProject(project.id);
      await load();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Could not delete the project.",
      );
    }
  }

  if (sessionLoading) {
    return <div className="spinner">Loading...</div>;
  }

  return (
    <>
      <div className="row between" style={{ marginBottom: 24 }}>
        <div>
          <h1>Projects</h1>
          <p className="subtitle" style={{ marginBottom: 0 }}>
            {user ? `Signed in as ${user.email}` : ""}
          </p>
        </div>
        <div className="row">
          <button onClick={() => setCreating((v) => !v)}>
            {creating ? "Cancel" : "New project"}
          </button>
          <button onClick={() => void signOut()}>Sign out</button>
        </div>
      </div>

      {error ? <div className="error">{error}</div> : null}
      {notice ? <div className="success">{notice}</div> : null}

      {creating ? (
        <div className="card" style={{ marginBottom: 20 }}>
          <h3 style={{ marginTop: 0 }}>New project</h3>
          <form onSubmit={handleCreate}>
            <div className="field">
              <label htmlFor="project-name">Name</label>
              <input
                id="project-name"
                type="text"
                required
                maxLength={200}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My Application"
              />
            </div>
            <div className="field">
              <label htmlFor="project-slug">Slug</label>
              <input
                id="project-slug"
                type="text"
                required
                maxLength={64}
                pattern="[a-z0-9\-]+"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="my-application"
              />
              <div className="hint">
                Lowercase letters, digits, and hyphens. The slug is a stable
                identifier and cannot be changed later.
              </div>
            </div>
            <div className="actions">
              <button className="primary" type="submit" disabled={submitting}>
                {submitting ? "Creating..." : "Create project"}
              </button>
            </div>
          </form>
        </div>
      ) : null}

      <div className="card">
        {loading ? (
          <div className="spinner">Loading projects...</div>
        ) : projects.length === 0 ? (
          <div className="empty">
            No projects yet. Create one to get started.
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Slug</th>
                <th style={{ width: 140 }} />
              </tr>
            </thead>
            <tbody>
              {projects.map((project) => (
                <tr key={project.id}>
                  <td>
                    <Link href={`/projects/${project.id}`}>
                      <strong>{project.name}</strong>
                    </Link>
                  </td>
                  <td>
                    <code>{project.slug}</code>
                  </td>
                  <td>
                    <div className="row" style={{ justifyContent: "flex-end" }}>
                      <Link href={`/projects/${project.id}`}>
                        <button className="small">Manage</button>
                      </Link>
                      <button
                        className="small danger"
                        onClick={() => void handleDelete(project)}
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
    </>
  );
}
