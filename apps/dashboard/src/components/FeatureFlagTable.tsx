"use client";

import { useCallback, useRef, useState } from "react";
import type { FlagValue } from "@/lib/api";
import { CopyButton, StatusBadge } from "./primitives";

/**
 * The feature flag toggle.
 *
 * The control shows a loading state while the request runs, prevents
 * duplicate requests, and only reflects the new state after the API
 * confirms it.
 */
export function FeatureFlagToggle({
  flag,
  environmentName,
  isProduction,
  onChange,
  onError,
  onRequestStart,
}: {
  flag: FlagValue;
  environmentName: string;
  isProduction: boolean;
  onChange: (flag: FlagValue, enabled: boolean) => Promise<void>;
  onError: (message: string) => void;
  onRequestStart?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);

  const handleChange = useCallback(
    async (event: React.ChangeEvent<HTMLInputElement>) => {
      const enabled = event.target.checked;

      // Ignore the change while a request is already running, so a
      // double click cannot send two writes.
      if (inFlight.current) {
        event.target.checked = !enabled;
        return;
      }

      inFlight.current = true;
      setBusy(true);
      onRequestStart?.();

      try {
        await onChange(flag, enabled);
      } catch (error) {
        // Put the control back the way it was.
        event.target.checked = !enabled;
        onError(
          error instanceof Error
            ? error.message
            : "The flag could not be updated.",
        );
      } finally {
        inFlight.current = false;
        setBusy(false);
      }
    },
    [flag, onChange, onError, onRequestStart],
  );

  return (
    <div className="row" style={{ gap: "var(--space-2)" }}>
      <label className="toggle">
        <input
          type="checkbox"
          checked={flag.enabled}
          disabled={busy}
          onChange={(event) => void handleChange(event)}
          aria-label={`${flag.key} in ${environmentName}`}
        />
        <span className="toggle-track" />
        <span className="toggle-thumb" />
        {busy ? <span className="toggle-busy" /> : null}
      </label>
      <StatusBadge tone={flag.enabled ? "on" : "off"}>
        {flag.enabled ? "ON" : "OFF"}
      </StatusBadge>
      {isProduction ? (
        <span className="sr-only">This change affects production.</span>
      ) : null}
    </div>
  );
}
