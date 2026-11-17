"use client";

import { PageHeader } from "@/components/primitives";
import { useSession } from "@/lib/useSession";

/**
 * The account settings.
 *
 * The MVP has no organisation, billing, or role management, so this
 * shows only what actually exists: the signed in administrator and how
 * to leave.
 */
export default function SettingsPage() {
  const { user } = useSession();

  return (
    <>
      <PageHeader
        title="Settings"
        description="Account details for this dashboard."
      />

      <div className="card">
        <div className="card-header">
          <h2 className="card-title">Administrator</h2>
        </div>
        <div className="card-body">
          <div className="field">
            <label htmlFor="settings-email">Email</label>
            <input
              id="settings-email"
              type="email"
              value={user?.email ?? ""}
              readOnly
              aria-readonly="true"
            />
            <p className="field-hint">
              The administrator account is created with the setup command. Its
              email cannot be changed from the dashboard.
            </p>
          </div>
        </div>
      </div>
    </>
  );
}
