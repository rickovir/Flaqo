"use client";

import { useCallback, useEffect, useState } from "react";

/** Page header: title, description, and the primary contextual action. */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <header className="page-header">
      <div className="page-title-group">
        <h1>{title}</h1>
        {description ? (
          <p className="page-description">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="page-actions">{actions}</div> : null}
    </header>
  );
}

/** Breadcrumb trail. */
export function Breadcrumbs({
  items,
}: {
  items: Array<{ label: string; href?: string }>;
}) {
  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      {items.map((item, index) => {
        const isLast = index === items.length - 1;
        return (
          <span key={`${item.label}-${index}`}>
            {index > 0 ? (
              <span className="separator" aria-hidden="true">
                {" / "}
              </span>
            ) : null}
            {item.href && !isLast ? (
              <a href={item.href}>{item.label}</a>
            ) : (
              <span aria-current={isLast ? "page" : undefined}>{item.label}</span>
            )}
          </span>
        );
      })}
    </nav>
  );
}

/** A status badge. The label always carries the meaning, never colour alone. */
export function StatusBadge({
  children,
  tone = "neutral",
}: {
  children: React.ReactNode;
  tone?: "neutral" | "on" | "off" | "revoked" | "warning";
}) {
  const className =
    tone === "on"
      ? "badge badge-on"
      : tone === "revoked"
        ? "badge badge-revoked"
        : tone === "warning"
          ? "badge badge-warning"
          : tone === "off"
            ? "badge badge-off"
            : "badge";

  return <span className={className}>{children}</span>;
}

/** An empty state that explains what is missing and offers the action. */
export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="empty-state">
      <p className="empty-title">{title}</p>
      {description ? <p className="empty-description">{description}</p> : null}
      {action}
    </div>
  );
}

/** A skeleton placeholder for a table row. */
export function SkeletonRows({
  rows = 3,
  columns = 1,
}: {
  rows?: number;
  columns?: number;
}) {
  return (
    <div aria-hidden="true">
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div
          key={rowIndex}
          style={{ display: "flex", gap: "var(--space-4)", padding: "var(--space-3) var(--space-4)" }}
        >
          {Array.from({ length: columns }).map((__, columnIndex) => (
            <div
              key={columnIndex}
              className="skeleton skeleton-row"
              style={{ flex: columnIndex === 0 ? "1" : "0 0 120px" }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

/** A loading state with an accessible announcement. */
export function LoadingState({
  label = "Loading",
  rows,
  columns,
}: {
  label?: string;
  rows?: number;
  columns?: number;
}) {
  return (
    <div role="status" aria-live="polite">
      <span className="sr-only">{label}</span>
      {rows ? <SkeletonRows rows={rows} columns={columns} /> : null}
      {!rows ? <p className="text-secondary text-metadata">{label}</p> : null}
    </div>
  );
}

/** An error state with a retry action where retrying makes sense. */
export function ErrorState({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="error-state" role="alert">
      <div className="error-state-message">{message}</div>
      {onRetry ? (
        <button type="button" className="btn btn-sm" onClick={onRetry}>
          Retry
        </button>
      ) : null}
    </div>
  );
}

/** Copies text to the clipboard and confirms it briefly. */
export function CopyButton({
  value,
  label = "Copy",
}: {
  value: string;
  label?: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      // Clipboard access can be denied. The value stays selectable, so
      // the failure is not silent in effect.
      setCopied(false);
    }
  }, [value]);

  return (
    <button
      type="button"
      className="btn btn-icon"
      onClick={() => void copy()}
      aria-label={copied ? "Copied" : label}
      title={copied ? "Copied" : label}
    >
      {copied ? (
        <svg
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          aria-hidden="true"
        >
          <path
            d="M2 7.5l3.5 3.5L12 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      ) : (
        <svg
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          aria-hidden="true"
        >
          <rect
            x="4.5"
            y="4.5"
            width="8"
            height="8"
            rx="1.5"
            stroke="currentColor"
            strokeWidth="1.4"
          />
          <path
            d="M9.5 4.5v-1a1.5 1.5 0 00-1.5-1.5H3A1.5 1.5 0 001.5 3.5V9A1.5 1.5 0 003 10.5h1"
            stroke="currentColor"
            strokeWidth="1.4"
            strokeLinecap="round"
          />
        </svg>
      )}
    </button>
  );
}
