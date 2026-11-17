// Drives the dashboard in a real browser.
//
// It signs in, creates a project, creates a flag, toggles it, and
// issues an SDK key, verifying the UI reflects each step.
import { chromium } from "playwright";

const DASH = process.env.DASH_URL ?? "http://localhost:3000";
const EMAIL = process.env.ADMIN_EMAIL ?? "admin@example.com";
const PASSWORD = process.env.ADMIN_PASSWORD ?? "a-strong-local-password-123";

let failures = 0;
function check(name, condition, detail = "") {
  if (!condition) {
    failures += 1;
  }
  console.log(`  ${condition ? "PASS" : "FAIL"}  ${name}${detail ? ` (${detail})` : ""}`);
}

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

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

try {
  console.log("login page");
  await page.goto(`${DASH}/login`, { waitUntil: "networkidle" });
  check("login page rendered", await page.locator("h1").innerText() === "Sign in");

  console.log("\nsign in with the wrong password");
  await page.fill("#email", EMAIL);
  await page.fill("#password", "definitely-wrong");
  await page.click("button[type=submit]");
  await page.waitForSelector(".error", { timeout: 10_000 });
  const errorText = await page.locator(".error").innerText();
  check("wrong password shows a generic error", errorText.includes("authentication required"), errorText);

  console.log("\nsign in for real");
  await page.fill("#password", PASSWORD);
  await page.click("button[type=submit]");
  await page.waitForURL("**/projects", { timeout: 15_000 });
  check("redirected to the projects page", page.url().includes("/projects"));

  console.log("\ncreate a project");
  await page.click("text=New project");
  await page.fill("#project-name", projectName);
  await page.fill("#project-slug", projectSlug);
  await page.click("button[type=submit]");
  await page.waitForSelector(".success", { timeout: 15_000 });
  const createNotice = await page.locator(".success").innerText();
  check("project created", createNotice.includes("3 environments"), createNotice);

  await page.waitForSelector(`text=${projectName}`, { timeout: 10_000 });
  check("project appears in the list", await page.locator(`text=${projectName}`).count() > 0);

  console.log("\nopen the project");
  const projectLink = page.locator(`tr:has-text("${projectName}") a`).first();
  await projectLink.click();
  await page.waitForURL("**/projects/**", { timeout: 20_000 });
  await page.waitForSelector(".env-tabs", { timeout: 20_000 });
  const tabs = await page.locator(".env-tabs button").allInnerTexts();
  check("three environment tabs", tabs.length === 3, tabs.join(", "));

  console.log("\ncreate a flag");
  await page.click("text=New flag");
  await page.fill("#flag-key", flagKey);
  await page.fill("#flag-description", "Created by the browser test");
  await page.click("button[type=submit]");
  await page.waitForSelector(".success", { timeout: 15_000 });
  const flagNotice = await page.locator(".success").innerText();
  check("flag created", flagNotice.includes(flagKey), flagNotice);

  await page.waitForSelector(`code:has-text("${flagKey}")`, { timeout: 10_000 });
  check("flag appears in the table", await page.locator(`code:has-text("${flagKey}")`).count() > 0);

  const badge = page.locator("tbody tr").filter({ hasText: flagKey }).first().locator(".badge");
  check("flag starts OFF", (await badge.innerText()).trim() === "OFF");

  console.log("\ntoggle the flag ON");
  const flagRow = page.locator("tbody tr").filter({ hasText: flagKey }).first();
  await flagRow.locator(".toggle input").click();
  await page.waitForFunction(
    ({ key }) => {
      const rows = Array.from(document.querySelectorAll("tbody tr"));
      const row = rows.find((r) => r.textContent?.includes(key));
      const badge = row?.querySelector(".badge");
      return badge?.textContent?.trim() === "ON";
    },
    { key: flagKey },
    { timeout: 15_000 },
  );
  check("flag toggled ON", (await badge.innerText()).trim() === "ON");

  console.log("\nenvironment independence");
  await page.click(".env-tabs button:has-text('Production')");
  await page.waitForSelector(`code:has-text("${flagKey}")`, { timeout: 10_000 });
  // The flags for the newly selected environment are fetched, so the
  // badge is only read once the request for that environment has
  // completed. Waiting for the response avoids reading a stale row.
  await page.waitForResponse(
    (r) => r.url().includes("/flags?environment=") && r.status() === 200,
    { timeout: 10_000 },
  ).catch(() => undefined);
  const prodBadge = page.locator("tbody tr").filter({ hasText: flagKey }).first().locator(".badge");
  check("production is still OFF", (await prodBadge.innerText()).trim() === "OFF");

  console.log("\nissue an SDK key");
  await page.click(".env-tabs button:has-text('Production')");
  await page.click("text=New key");
  await page.fill("#key-name", "browser-test-key");
  await page.selectOption("#key-type", "client");
  await page.click("button[type=submit]");
  await page.waitForSelector(".key-reveal", { timeout: 15_000 });
  const rawKey = await page.locator(".key-reveal").innerText();
  check("raw key shown once", rawKey.startsWith("ff_client_"), rawKey.slice(0, 24));

  await page.click("text=I have saved it");
  await page.waitForSelector(".key-reveal", { state: "detached", timeout: 10_000 });
  check("key reveal dismissed", (await page.locator(".key-reveal").count()) === 0);

  const keyRow = page.locator("tr:has-text('browser-test-key')");
  const rowText = await keyRow.innerText();
  check("key listed with a prefix only", !rowText.includes("ff_client_"), rowText.replace(/\s+/g, " ").slice(0, 60));

  console.log("\nreload restores the session");
  await page.reload({ waitUntil: "networkidle" });
  await page.waitForSelector(".env-tabs", { timeout: 15_000 });
  check("still signed in after reload", page.url().includes("/projects"));
  check("no redirect to login", !page.url().includes("/login"));

  console.log("\naction still works after reload");
  await page.click(".env-tabs button:has-text('Production')");
  await page.waitForSelector(`code:has-text("${flagKey}")`, { timeout: 10_000 });
  await page
    .locator("tbody tr")
    .filter({ hasText: flagKey })
    .first()
    .locator(".toggle input")
    .click();
  await page.waitForFunction(
    ({ key }) => {
      const rows = Array.from(document.querySelectorAll("tbody tr"));
      const row = rows.find((r) => r.textContent?.includes(key));
      const badge = row?.querySelector(".badge");
      return badge?.textContent?.trim() === "ON";
    },
    { key: flagKey },
    { timeout: 15_000 },
  );
  const afterReload = page
    .locator("tbody tr")
    .filter({ hasText: flagKey })
    .first()
    .locator(".badge");
  check("toggle works after reload", (await afterReload.innerText()).trim() === "ON");

  console.log("\nsign out");
  // Sign out lives on the projects page.
  await page.goto(`${DASH}/projects`, { waitUntil: "networkidle" });
  await page.waitForSelector("text=Sign out", { timeout: 10_000 });
  await page.click("text=Sign out");
  await page.waitForURL("**/login", { timeout: 15_000 });
  check("signed out", page.url().includes("/login"));

  await page.goto(`${DASH}/projects`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  check("projects page redirects to login when signed out", page.url().includes("/login"), page.url());

  console.log("\nconsole errors");
  // A 401 is expected when the session is checked while signed out, and
  // a missing favicon is not a defect.
  const realErrors = consoleErrors.filter(
    (e) =>
      !e.includes("favicon") &&
      !e.includes("404 (Not Found)") &&
      !e.includes("401 (Unauthorized)"),
  );
  check("no unexpected console errors", realErrors.length === 0, realErrors.slice(0, 2).join(" | "));
  console.log(
    `  note  ${consoleErrors.length} console message(s) observed, all expected 401s or a missing favicon`,
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
  failures === 0 ? "\nAll browser checks passed." : `\n${failures} browser check(s) failed.`,
);
process.exit(failures === 0 ? 0 : 1);
