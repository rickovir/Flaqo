/**
 * Browser client for the dashboard API.
 *
 * The API authenticates with a session cookie, and every state-changing
 * request must carry the session-bound CSRF token. The token is kept in
 * memory after login and refreshed from the auth/me endpoint when the
 * page reloads.
 *
 * The CSRF token is never stored in localStorage. It is readable by the
 * dashboard's own JavaScript, which is what the double submit mechanism
 * requires, but it is not a credential that grants access on its own.
 */

export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

/** An error returned by the API, carrying its stable error code. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }

  /** Returns true when the request was not authenticated. */
  get isUnauthorized(): boolean {
    return this.status === 401;
  }

  /** Returns true when the request failed CSRF validation. */
  get isForbidden(): boolean {
    return this.status === 403;
  }
}

/**
 * The session-bound CSRF token.
 *
 * It is held in memory and mirrored in a readable cookie by the API, so
 * it can be recovered after a page reload without a round trip.
 */
let csrfToken: string | undefined;

/** The name of the readable cookie holding the CSRF token. */
const CSRF_COOKIE_NAME = "ff_csrf";

/** Returns the current CSRF token, if one has been obtained. */
export function getCsrfToken(): string | undefined {
  if (csrfToken) {
    return csrfToken;
  }
  // After a page reload the in-memory value is gone, so it is read back
  // from the cookie the API set at login.
  if (typeof document !== "undefined") {
    const match = document.cookie
      .split(";")
      .map((part) => part.trim())
      .find((part) => part.startsWith(`${CSRF_COOKIE_NAME}=`));
    if (match) {
      csrfToken = decodeURIComponent(match.slice(CSRF_COOKIE_NAME.length + 1));
    }
  }
  return csrfToken;
}

/** Stores the CSRF token for subsequent state-changing requests. */
export function setCsrfToken(token: string | undefined): void {
  csrfToken = token;
}

/**
 * Performs an API request.
 *
 * The session cookie is included, and the CSRF token is attached to
 * state-changing requests.
 */
async function request<T>(
  path: string,
  init: RequestInit & { method?: string } = {},
): Promise<T> {
  const method = init.method ?? "GET";
  const headers = new Headers(init.headers);

  if (init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  // Sign in is the one state-changing request that cannot carry a CSRF
  // token, because the token is issued by signing in. Every other
  // mutation requires it.
  const needsCsrf =
    method !== "GET" && method !== "HEAD" && !path.endsWith("/auth/login");

  if (needsCsrf) {
    const token = getCsrfToken();
    if (!token) {
      throw new ApiError(
        403,
        "missing_csrf_token",
        "No CSRF token is available. Sign in again.",
      );
    }
    headers.set("X-CSRF-Token", token);
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    method,
    headers,
    // The session cookie must be sent with every request.
    credentials: "include",
  });

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  let body: unknown = undefined;
  if (text.length > 0) {
    try {
      body = JSON.parse(text);
    } catch {
      body = undefined;
    }
  }

  if (!response.ok) {
    const errorBody = (body as { error?: { code?: string; message?: string } })?.error;
    throw new ApiError(
      response.status,
      errorBody?.code ?? "unknown_error",
      errorBody?.message ?? `Request failed with status ${response.status}`,
    );
  }

  return body as T;
}

/** Signs an administrator in. */
export async function login(
  email: string,
  password: string,
): Promise<{ user_id: string; email: string; csrf_token: string }> {
  const result = await request<{
    user_id: string;
    email: string;
    csrf_token: string;
  }>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });

  csrfToken = result.csrf_token;
  return result;
}

/** Signs the current administrator out. */
export async function logout(): Promise<void> {
  try {
    await request<void>("/api/v1/auth/logout", { method: "POST" });
  } finally {
    csrfToken = undefined;
  }
}

/** The current administrator. */
export interface CurrentUser {
  user_id: string;
  email: string;
  csrf_token?: string;
  expires_at?: string;
}

/**
 * Returns the current administrator.
 *
 * The CSRF token is refreshed from this response, so a page reload
 * restores the ability to make state-changing requests.
 */
export async function me(): Promise<CurrentUser | undefined> {
  try {
    const result = await request<CurrentUser>("/api/v1/auth/me");
    if (result.csrf_token) {
      csrfToken = result.csrf_token;
    }
    return result;
  } catch (error) {
    if (error instanceof ApiError && error.isUnauthorized) {
      csrfToken = undefined;
      return undefined;
    }
    throw error;
  }
}

/** A project. */
export interface Project {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

/** An environment. */
export interface Environment {
  id: string;
  project_id: string;
  key: string;
  name: string;
  config_version: number;
}

/** Lists the administrator's projects. */
export async function listProjects(): Promise<Project[]> {
  const result = await request<{ projects: Project[] }>("/api/v1/projects");
  return result.projects;
}

/** Creates a project. */
export async function createProject(
  name: string,
  slug: string,
): Promise<{ project: Project; environments: Environment[] }> {
  return request("/api/v1/projects", {
    method: "POST",
    body: JSON.stringify({ name, slug }),
  });
}

/** Returns one project. */
export async function getProject(projectId: string): Promise<Project> {
  const result = await request<{ project: Project }>(
    `/api/v1/projects/${projectId}`,
  );
  return result.project;
}

/** Updates a project. */
export async function updateProject(
  projectId: string,
  name: string,
): Promise<Project> {
  const result = await request<{ project: Project }>(
    `/api/v1/projects/${projectId}`,
    { method: "PATCH", body: JSON.stringify({ name }) },
  );
  return result.project;
}

/** Deletes a project. */
export async function deleteProject(projectId: string): Promise<void> {
  await request<void>(`/api/v1/projects/${projectId}`, { method: "DELETE" });
}

/** Lists a project's environments. */
export async function listEnvironments(
  projectId: string,
): Promise<Environment[]> {
  const result = await request<{ environments: Environment[] }>(
    `/api/v1/projects/${projectId}/environments`,
  );
  return result.environments;
}

/** A flag with its state in the selected environment. */
export interface FlagValue {
  id: string;
  key: string;
  enabled: boolean;
}

/** Lists a project's flags in one environment. */
export async function listFlags(
  projectId: string,
  environmentId: string,
): Promise<FlagValue[]> {
  const result = await request<{ flags: FlagValue[] }>(
    `/api/v1/projects/${projectId}/flags?environment=${environmentId}`,
  );
  return result.flags;
}

/** Creates a flag. */
export async function createFlag(
  projectId: string,
  key: string,
  description?: string,
): Promise<void> {
  await request(`/api/v1/projects/${projectId}/flags`, {
    method: "POST",
    body: JSON.stringify({ key, description: description ?? "" }),
  });
}

/** Updates a flag's description. */
export async function updateFlagDescription(
  flagId: string,
  description: string,
): Promise<void> {
  await request(`/api/v1/flags/${flagId}`, {
    method: "PATCH",
    body: JSON.stringify({ key: "", description }),
  });
}

/** Deletes a flag. */
export async function deleteFlag(flagId: string): Promise<void> {
  await request<void>(`/api/v1/flags/${flagId}`, { method: "DELETE" });
}

/** Sets a flag's state in one environment. */
export async function setFlagState(
  flagId: string,
  environmentId: string,
  enabled: boolean,
): Promise<{ version: number; changed: boolean }> {
  return request(`/api/v1/flags/${flagId}/environments/${environmentId}`, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

/** An SDK key. */
export interface SdkKey {
  id: string;
  project_id: string;
  environment_id: string;
  name: string;
  key_type: "client" | "server";
  key_prefix: string;
  revoked_at: string | null;
  created_at: string;
}

/** Lists an environment's SDK keys. */
export async function listSdkKeys(
  projectId: string,
  environmentId: string,
): Promise<SdkKey[]> {
  const result = await request<{ keys: SdkKey[] }>(
    `/api/v1/sdk-keys?project_id=${projectId}&environment=${environmentId}`,
  );
  return result.keys;
}

/**
 * Issues an SDK key.
 *
 * The raw key is returned once and is not retrievable afterwards.
 */
export async function createSdkKey(
  projectId: string,
  environmentId: string,
  name: string,
  keyType: "client" | "server",
): Promise<{ key: string; key_prefix: string }> {
  return request("/api/v1/sdk-keys", {
    method: "POST",
    body: JSON.stringify({
      project_id: projectId,
      environment_id: environmentId,
      name,
      key_type: keyType,
    }),
  });
}

/** Revokes an SDK key permanently. */
export async function revokeSdkKey(
  projectId: string,
  keyId: string,
): Promise<void> {
  await request<void>(`/api/v1/sdk-keys/${keyId}?project_id=${projectId}`, {
    method: "DELETE",
  });
}
