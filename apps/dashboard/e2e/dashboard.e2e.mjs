// Drives the dashboard in a real browser.
//
// It signs in, creates a project, creates a flag, toggles it, checks the
// Production guard, issues an SDK key, and verifies the session survives
// a reload.
//
// It needs a running API and dashboard:
//   API      http://localhost:8080
//   DASHBOARD http://localhost:3000
//
// Run with: npm run e2e
import { chromium } from "playwright";

const DASH = process.env.DASH_URL ?? "http://localhost:3000";
const API = process.env.API_URL ?? "http://localhost:8080";
const EMAIL = process.env.ADMIN_EMAIL ?? "admin@example.com";
const PASSWORD = process.env.ADMIN_PASSWORD ?? "a-strong-local-password-123";

let failures = 0;
function check(name, condition, detail = "") {
  if (!condition) {
    failures += 1;
  }
  console.log(
    `  ${condition ? "PASS" : "FAIL"}  ${name}${detail ? ` (${detail})` : ""}`,
  );
}

const browser = await chromium.launch();
const page = await browser.newPage({
  viewport: { width: 1440, height: 900 },
});

const consoleErrors = [];
page.on("console", (msg) => {
  if (msg.type() === "error") {
    consoleErrors.push(msg.text());
  }
});
page.on("pageerror", (err) => consoleErrors.push(String(err)));

const suffix = Date.now().toString(36).slice(-6);
const projectName = `Browser ${suffix}`;
const projectSlug = `browser-${suffix}`;
const flagKey = `browser_flag_${suffix}`;

/** The badge text for a row containing the given key. */
async function badgeFor(key) {
  return page
    .locator("tbody tr")
    .filter({ hasText: key })
    .first()
    .locator(".badge")
    .innerText();
}

/** Waits until the badge for a key reports a given state. */
async function waitForBadge(key, expected) {
  await page.waitForFunction(
    ({ flagKey: k, expected: want }) => {
      const rows = Array.from(document.querySelectorAll("tbody tr"));
      const row = rows.find((r) => r.textContent?.includes(k));
      return row?.querySelector(".badge")?.textContent?.trim() === want;
    },
    { flagKey: key, expected },
    { timeout: 15_000 },
  );
}

try {
  console.log("login page");
  await page.goto(`${DASH}/login`, { waitUntil: "networkidle" });
  check(
    "login page rendered",
    (await page.locator("h1").innerText()) === "Sign in",
  );
  check(
    "no sidebar on the login page",
    (await page.locator(".sidebar").count()) === 0,
  );

  console.log("\nsign in with the wrong password");
  await page.fill("#email", EMAIL);
  await page.fill("#password", "definitely-wrong");
  await page.click("button[type=submit]");
  await page.waitForSelector(".error-state", { timeout: 10_000 });
  const errorText = await page.locator(".error-state").innerText();
  check(
    "wrong password shows a generic error",
    errorText.includes("authentication required"),
    errorText,
  );

  console.log("\nsign in for real");
  await page.fill("#password", PASSWORD);
  await page.click("button[type=submit]");
  await page.waitForURL("**/projects", { timeout: 15_000 });
  check("redirected to the projects page", page.url().includes("/projects"));

  console.log("\nthe sidebar is present");
  // The sidebar renders once the session resolves, so it is awaited.
  await page.waitForSelector(".sidebar", { timeout: 15_000 });
  check("sidebar rendered", (await page.locator(".sidebar").count()) === 1);
  check(
    "workspace navigation present",
    (await page.locator(".sidebar-nav .nav-link").count()) >= 4,
  );
  check(
    "active navigation item is marked",
    (await page.locator('.nav-link[aria-current="page"]').count()) >= 1,
  );

  console.log("\ncreate a project");
  await page.click("text=New project");
  await page.waitForSelector('[role="dialog"]', { timeout: 10_000 });
  check("create project opens a dialog", (await page.locator('[role="dialog"]').count()) === 1);

  await page.fill("#dialog-project-name", projectName);
  await page.fill("#dialog-project-slug", projectSlug);
  await page.click("button[type=submit]");
  await page.waitForSelector('[role="dialog"]', { state: "detached", timeout: 15_000 });
  check("dialog closed after submit", (await page.locator('[role="dialog"]').count()) === 0);

  await page.waitForSelector(`tr:has-text("${projectName}")`, { timeout: 15_000 });
  check(
    "project appears in the list",
    (await page.locator(`tr:has-text("${projectName}")`).count()) > 0,
  );

  console.log("\nopen the project");
  await page.locator(`tr:has-text("${projectName}") a`).first().click();
  await page.waitForURL("**/projects/**", { timeout: 20_000 });
  await page.waitForSelector(".env-selector", { timeout: 20_000 });
  check("breadcrumb present", (await page.locator(".breadcrumbs").count()) === 1);

  const tabs = await page.locator('.env-selector [role="tab"]').allInnerTexts();
  check("three environment tabs", tabs.length === 3, tabs.join(", "));
  check(
    "tabs are a tablist",
    (await page.locator('[role="tablist"]').count()) === 1,
  );
  const normalizedTabs = tabs.map((t) => t.replace(/\s+/g, " ").trim());
  check(
    "tab version is separated from the name",
    normalizedTabs.every((t) => {
      const parts = t.split(" ");
      const version = parts[parts.length - 1] ?? "";
      return parts.length >= 2 && version.startsWith("v") && version.length > 1;
    }),
    normalizedTabs.join(", "),
  );

  console.log("\ncreate a flag");
  await page.click("text=New flag");
  await page.waitForSelector('[role="dialog"]', { timeout: 10_000 });
  await page.fill("#dialog-flag-key", flagKey);
  await page.fill("#dialog-flag-description", "Created by the browser test");
  await page.click("button[type=submit]");
  await page.waitForSelector('[role="dialog"]', { state: "detached", timeout: 15_000 });

  await page.waitForSelector(`code:has-text("${flagKey}")`, { timeout: 15_000 });
  check("flag appears in the table", (await badgeFor(flagKey)) === "OFF");

  console.log("\ntoggle the flag ON");
  await page
    .locator("tbody tr")
    .filter({ hasText: flagKey })
    .first()
    .locator(".toggle input")
    .click();
  await waitForBadge(flagKey, "ON");
  check("flag toggled ON", (await badgeFor(flagKey)) === "ON");

  console.log("\na toast confirms the change");
  await page.waitForSelector(".toast", { timeout: 10_000 });
  const toastText = await page.locator(".toast").last().innerText();
  check("toast shown", toastText.includes("is now ON"), toastText);
  check(
    "toast region is a live region",
    (await page.locator('.toast-region[aria-live="polite"]').count()) === 1,
  );

  console.log("\nenvironment independence");
  await page.click('.env-selector [role="tab"]:has-text("Production")');
  await page.waitForSelector(`code:has-text("${flagKey}")`, { timeout: 15_000 });
  // The flags for the newly selected environment are fetched, so the
  // badge is only read once that request has completed. Reading it
  // earlier would report the previous environment's row.
  await page
    .waitForResponse(
      (r) => r.url().includes("/flags?environment=") && r.status() === 200,
      { timeout: 10_000 },
    )
    .catch(() => undefined);
  check("production is still OFF", (await badgeFor(flagKey)) === "OFF");

  console.log("\nproduction changes are confirmed");
  check(
    "production notice shown",
    (await page.locator(".prod-notice").count()) === 1,
  );

  await page
    .locator("tbody tr")
    .filter({ hasText: flagKey })
    .first()
    .locator(".toggle input")
    .click();
  await page.waitForSelector('[role="dialog"]', { timeout: 10_000 });
  const dialogText = await page.locator('[role="dialog"]').innerText();
  check("confirmation dialog opened", dialogText.includes("reaches live traffic"), dialogText.slice(0, 60));

  // Cancelling must not apply the change.
  await page.click('[role="dialog"] >> text=Cancel');
  await page.waitForSelector('[role="dialog"]', { state: "detached", timeout: 10_000 });
  await page.waitForTimeout(500);
  check("cancelling leaves the flag OFF", (await badgeFor(flagKey)) === "OFF");

  console.log("\nissue an SDK key");
  await page.click('.sidebar-nav >> text=SDK Keys');
  await page.waitForURL("**/sdk-keys", { timeout: 15_000 });
  await page.waitForSelector("text=Generate key", { timeout: 15_000 });
  check(
    "SDK keys are a separate page",
    (await page.locator("h1").innerText()) === "SDK keys",
  );

  await page.click("text=Generate key");
  await page.waitForSelector('[role="dialog"]', { timeout: 10_000 });
  await page.fill("#dialog-key-name", "browser-test-key");
  await page.selectOption("#dialog-key-type", "client");
  await page.click("button[type=submit]");
  await page.waitForSelector(".key-reveal", { timeout: 15_000 });

  const rawKey = await page.locator(".key-reveal-value").innerText();
  check("raw key shown once", rawKey.startsWith("ff_client_"), rawKey.slice(0, 24));
  check(
    "copy action provided",
    (await page.locator('.key-reveal button[aria-label="Copy SDK key"]').count()) === 1,
  );

  await page.click("text=I have saved it");
  await page.waitForSelector(".key-reveal", { state: "detached", timeout: 10_000 });
  check("key reveal dismissed", (await page.locator(".key-reveal").count()) === 0);

  // The key name is reused across runs, so the newest row is the last.
  const keyRow = await page
    .locator("tr:has-text('browser-test-key')")
    .last()
    .innerText();
  check(
    "key listed with a prefix only",
    !keyRow.includes("ff_client_"),
    keyRow.replace(/\s+/g, " ").slice(0, 60),
  );

  console.log("\nreload restores the session");
  await page.goto(`${DASH}/projects`, { waitUntil: "networkidle" });
  await page.waitForSelector("text=New project", { timeout: 15_000 });
  check("still signed in after reload", !page.url().includes("/login"));

  console.log("\nthe sidebar navigates");
  for (const [label, expected] of [
    ["Feature Flags", "**/feature-flags"],
    ["SDK Keys", "**/sdk-keys"],
    ["Settings", "**/settings"],
    ["Projects", "**/projects"],
  ]) {
    await page.click(`.sidebar-nav >> text=${label}`);
    await page.waitForURL(expected, { timeout: 15_000 });
    check(`${label} navigates`, page.url().endsWith(expected.replace("**", "")));
  }

  console.log("\nsign out");
  await page.click(".account");
  await page.waitForSelector('[role="menu"]', { timeout: 10_000 });

  // The menu must be inside the viewport, or it cannot be clicked.
  const menuBox = await page.locator('[role="menu"]').boundingBox();
  const viewport = page.viewportSize();
  check(
    "account menu is inside the viewport",
    menuBox !== null &&
      menuBox.y >= 0 &&
      menuBox.y + menuBox.height <= (viewport?.height ?? 0),
    menuBox ? `top=${Math.round(menuBox.y)}` : "no box",
  );

  await page.click('[role="menu"] >> text=Sign out');
  await page.waitForURL("**/login", { timeout: 15_000 });
  check("signed out", page.url().includes("/login"));

  await page.goto(`${DASH}/projects`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  check(
    "projects page redirects to login when signed out",
    page.url().includes("/login"),
    page.url(),
  );

  console.log("\nconsole errors");
  const realErrors = consoleErrors.filter(
    (e) =>
      !e.includes("favicon") &&
      !e.includes("404 (Not Found)") &&
      !e.includes("401 (Unauthorized)"),
  );
  check(
    "no unexpected console errors",
    realErrors.length === 0,
    realErrors.slice(0, 2).join(" | "),
  );
} catch (err) {
  failures += 1;
  console.log(`  FAIL  threw: ${err}`);
  await page.screenshot({ path: "/tmp/dashboard-failure.png", fullPage: true });
  console.log("  screenshot: /tmp/dashboard-failure.png");
} finally {
  await browser.close();
}

console.log(
  failures === 0
    ? "\nAll browser checks passed."
    : `\n${failures} browser check(s) failed.`,
);
process.exit(failures === 0 ? 0 : 1);
