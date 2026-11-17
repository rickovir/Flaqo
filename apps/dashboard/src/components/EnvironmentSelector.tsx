"use client";

import { useRef } from "react";
import type { Environment } from "@/lib/api";

/**
 * The environment selector.
 *
 * It is a proper tablist: the tabs carry the correct roles, arrow keys
 * move between them, and the version is separated from the name so the
 * accessible name reads cleanly.
 */
export function EnvironmentSelector({
  environments,
  selectedId,
  onSelect,
  panelId,
}: {
  environments: Environment[];
  selectedId: string | undefined;
  onSelect: (environmentId: string) => void;
  panelId: string;
}) {
  const listRef = useRef<HTMLDivElement>(null);

  function handleKeyDown(event: React.KeyboardEvent) {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") {
      return;
    }
    event.preventDefault();

    const index = environments.findIndex((env) => env.id === selectedId);
    if (index === -1) {
      return;
    }

    const offset = event.key === "ArrowRight" ? 1 : -1;
    const nextIndex =
      (index + offset + environments.length) % environments.length;
    const next = environments[nextIndex]!;
    onSelect(next.id);

    // Move focus with the selection.
    const tabs = listRef.current?.querySelectorAll<HTMLButtonElement>(
      '[role="tab"]',
    );
    tabs?.[nextIndex]?.focus();
  }

  return (
    <div
      className="env-selector"
      role="tablist"
      aria-label="Environment"
      ref={listRef}
      onKeyDown={handleKeyDown}
    >
      {environments.map((environment) => {
        const isProduction = environment.key === "production";
        const selected = environment.id === selectedId;

        return (
          <button
            key={environment.id}
            type="button"
            role="tab"
            id={`tab-${environment.id}`}
            aria-selected={selected}
            aria-controls={panelId}
            tabIndex={selected ? 0 : -1}
            className={`env-tab${isProduction ? " env-tab-production" : ""}`}
            onClick={() => onSelect(environment.id)}
          >
            <span>{environment.name}</span>
            <span className="env-tab-version">v{environment.config_version}</span>
          </button>
        );
      })}
    </div>
  );
}
