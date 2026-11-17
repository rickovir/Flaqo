"use client";

import { useEffect, useRef } from "react";

/**
 * Modal dialog with focus management.
 *
 * The dialog traps focus while it is open, closes on Escape, and
 * restores focus to the element that opened it. That behaviour is part
 * of the component so no caller can forget it.
 */
export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  labelledBy,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  labelledBy?: string;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }

    restoreFocusRef.current = document.activeElement as HTMLElement | null;

    // Move focus into the dialog, preferring the first control.
    const focusable = dialogRef.current?.querySelector<HTMLElement>(
      "input, textarea, select, button:not([disabled]), [href]",
    );
    (focusable ?? dialogRef.current)?.focus();

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }

      if (event.key !== "Tab") {
        return;
      }

      // Keep focus inside the dialog.
      const items = Array.from(
        dialogRef.current?.querySelectorAll<HTMLElement>(
          "input:not([disabled]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [href]",
        ) ?? [],
      ).filter((el) => el.offsetParent !== null);

      if (items.length === 0) {
        return;
      }

      const first = items[0]!;
      const last = items[items.length - 1]!;

      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown);

    return () => {
      document.removeEventListener("keydown", onKeyDown);
      // Focus returns to whatever opened the dialog.
      restoreFocusRef.current?.focus?.();
    };
  }, [open, onClose]);

  if (!open) {
    return null;
  }

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        // Clicking the backdrop closes, but a click inside the dialog
        // must not.
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        ref={dialogRef}
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy ?? "dialog-title"}
        tabIndex={-1}
      >
        <div className="dialog-header">
          <div>
            <h2 className="dialog-title" id={labelledBy ?? "dialog-title"}>
              {title}
            </h2>
            {description ? (
              <p className="dialog-description">{description}</p>
            ) : null}
          </div>
          <button
            type="button"
            className="btn btn-icon"
            onClick={onClose}
            aria-label="Close dialog"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              aria-hidden="true"
            >
              <path
                d="M1 1l12 12M13 1L1 13"
                stroke="currentColor"
                strokeWidth="1.6"
                strokeLinecap="round"
              />
            </svg>
          </button>
        </div>
        {children ? <div className="dialog-body">{children}</div> : null}
        {footer ? <div className="dialog-footer">{footer}</div> : null}
      </div>
    </div>
  );
}
