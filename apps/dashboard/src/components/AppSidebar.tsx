"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { BrandMark } from "@/components/Brand";
import { useSession } from "@/lib/useSession";

/** A small inline icon set, sized to the current text. */
function Icon({ path }: { path: string }) {
  return (
    <svg
      className="nav-icon"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      aria-hidden="true"
    >
      <path
        d={path}
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

const ICONS = {
  projects:
    "M2 4.5a1 1 0 011-1h3.2a1 1 0 01.8.4l.7.9a1 1 0 00.8.4H13a1 1 0 011 1v6.3a1 1 0 01-1 1H3a1 1 0 01-1-1V4.5z",
  flags: "M4 2v12M4 3.5h7.5l-1.5 2.5 1.5 2.5H4",
  keys:
    "M10.5 3.5a2.5 2.5 0 11-3.2 4.1L3 11.9v1.6h1.6v-1.5h1.5v-1.5h1.4l1.3-1.3a2.5 2.5 0 014-4.6z",
  settings:
    "M8 5.5a2.5 2.5 0 100 5 2.5 2.5 0 000-5zM8 1.5v1.6M8 12.9v1.6M14.5 8h-1.6M3.1 8H1.5M12.6 3.4l-1.1 1.1M4.5 11.5l-1.1 1.1M12.6 12.6l-1.1-1.1M4.5 4.5L3.4 3.4",
  logout: "M6 14H3.5a1 1 0 01-1-1V3a1 1 0 011-1H6M10 11l3-3-3-3M13 8H6",
  chevron: "M4 6l4 4 4-4",
} as const;

/**
 * The persistent application sidebar.
 *
 * It carries the workspace navigation, the active indicator, and the
 * account menu with the logout action.
 */
export function AppSidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { user, signOut } = useSession();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close the account menu when the pointer goes elsewhere.
  useEffect(() => {
    if (!menuOpen) {
      return;
    }
    function onPointerDown(event: MouseEvent) {
      if (!menuRef.current?.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuOpen]);

  const initials = (user?.email ?? "")
    .split("@")[0]!
    .slice(0, 2)
    .toUpperCase();

  // The project detail pages belong to Feature Flags.
  const flagsActive = pathname.startsWith("/projects/");

  return (
    <aside className="sidebar">
      <Link className="sidebar-brand" href="/projects">
        <BrandMark priority />
        <Image
          className="brand-wordmark-image"
          src="/brand/flaqo-wordmark-art.png"
          alt="Flaqo"
          width={96}
          height={35}
          priority
        />
      </Link>

      <nav className="sidebar-nav" aria-label="Workspace">
        <div className="nav-section">
          <p className="nav-section-label">Workspace</p>
          <Link
            className="nav-link"
            href="/projects"
            aria-current={pathname === "/projects" ? "page" : undefined}
          >
            <Icon path={ICONS.projects} />
            <span>Projects</span>
          </Link>
          <Link
            className="nav-link"
            href="/feature-flags"
            aria-current={
              flagsActive ? "page" : pathname === "/feature-flags" ? "page" : undefined
            }
          >
            <Icon path={ICONS.flags} />
            <span>Feature Flags</span>
          </Link>
          <Link
            className="nav-link"
            href="/sdk-keys"
            aria-current={pathname === "/sdk-keys" ? "page" : undefined}
          >
            <Icon path={ICONS.keys} />
            <span>SDK Keys</span>
          </Link>
        </div>

        <div className="nav-section">
          <p className="nav-section-label">Preferences</p>
          <Link className="nav-link" href="/settings">
            <Icon path={ICONS.settings} />
            <span>Settings</span>
          </Link>
        </div>
      </nav>

      <div className="sidebar-footer" ref={menuRef}>
        <button
          type="button"
          className="account"
          onClick={() => setMenuOpen((open) => !open)}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
        >
          <span className="account-avatar" aria-hidden="true">
            {initials || "?"}
          </span>
          <span className="account-text">
            <span className="account-email">{user?.email ?? "Signed out"}</span>
          </span>
          <Icon path={ICONS.chevron} />
        </button>

        {menuOpen ? (
          <div className="account-menu" role="menu" aria-label="Account">
            <button
              type="button"
              role="menuitem"
              className="btn btn-ghost btn-sm"
              style={{ width: "100%", justifyContent: "flex-start" }}
              onClick={() => {
                setMenuOpen(false);
                void signOut().then(() => router.push("/login"));
              }}
            >
              <Icon path={ICONS.logout} />
              Sign out
            </button>
          </div>
        ) : null}
      </div>
    </aside>
  );
}
