"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { AppSidebar } from "@/components/AppSidebar";
import { useSession } from "@/lib/useSession";

/**
 * The shell for the authenticated pages.
 *
 * It redirects to the sign in page when there is no session, so no
 * protected page can render for an unauthenticated visitor.
 */
export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { user, loading } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  if (loading || !user) {
    return (
      <div className="login-page">
        <div role="status" aria-live="polite">
          <span className="sr-only">Loading</span>
          <p className="text-secondary text-metadata">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="app">
      <AppSidebar />
      <div className="content">
        <div className="content-inner">{children}</div>
      </div>
    </div>
  );
}
