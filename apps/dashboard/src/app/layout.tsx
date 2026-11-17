import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Feature Flag Dashboard",
  description: "Manage Boolean feature flags per project and environment",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <div className="layout">
          <header className="header">
            <a className="brand" href="/">
              Feature Flags
            </a>
            <nav className="nav">
              <a href="/projects">Projects</a>
            </nav>
          </header>
          <main className="main">{children}</main>
        </div>
      </body>
    </html>
  );
}
