"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ApiError, me, type CurrentUser } from "@/lib/api";

/**
 * Loads the current session and provides a sign out action.
 *
 * The session cookie is read by the browser, so the dashboard never
 * handles the token itself.
 */
export function useSession() {
  const router = useRouter();
  const [user, setUser] = useState<CurrentUser | undefined>();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    me()
      .then((result) => {
        if (!cancelled) {
          setUser(result);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setUser(undefined);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const signOut = useCallback(async () => {
    try {
      const { logout } = await import("@/lib/api");
      await logout();
    } catch (error) {
      // A failed sign out still clears the local session state.
      if (!(error instanceof ApiError)) {
        // Nothing else to do: the local state is cleared regardless.
      }
    }
    setUser(undefined);
    router.push("/login");
  }, [router]);

  return { user, loading, signOut };
}

/**
 * Redirects to the login page when there is no session.
 *
 * It waits for the session check to finish before deciding, so a valid
 * session is not discarded by a redirect that ran too early.
 */
export function useRequireSession() {
  const { user, loading } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  return { user, loading };
}
