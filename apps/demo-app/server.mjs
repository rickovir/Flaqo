// A minimal application that uses the feature flag SDK.
//
// It serves a page that shows the current flag values, and it updates
// live when a flag changes in the dashboard. Nothing is redeployed and
// the page is not reloaded.
//
//   node server.mjs
import { createServer } from "node:http";
import { createFeatureFlagClient } from "@featureflag/sdk";

const apiUrl = process.env.API_URL ?? "http://localhost:8080";
const sdkKey = process.env.SDK_KEY;
const port = Number(process.env.PORT ?? 4000);

if (!sdkKey) {
  console.error("Set SDK_KEY to an SDK key from the dashboard.");
  console.error("  SDK_KEY=ff_client_... node server.mjs");
  process.exit(1);
}

const client = createFeatureFlagClient({ apiUrl, sdkKey });

console.log(`connecting to ${apiUrl}...`);
await client.initialize();

const snapshot = client.getSnapshot();
console.log(`connected. environment=${snapshot.environment} version=${snapshot.version}`);

// The page shows the flag values. A change event re-renders it.
function renderFlags() {
  const current = client.getSnapshot();
  if (!current) {
    return "<p>No configuration yet.</p>";
  }

  const rows = Object.entries(current.flags)
    .map(
      ([key, enabled]) => `
      <tr>
        <td><code>${escapeHtml(key)}</code></td>
        <td>
          <span class="flag ${enabled ? "on" : "off"}">
            ${enabled ? "ON" : "OFF"}
          </span>
        </td>
        <td><code>isEnabled("${escapeHtml(key)}")</code></td>
        <td><code>${enabled}</code></td>
      </tr>`,
    )
    .join("");

  return `
    <table>
      <thead>
        <tr><th>Flag</th><th>State</th><th>Call</th><th>Result</th></tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

const PAGE = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Feature Flag Demo</title>
<style>
  :root {
    --bg: #ffffff; --surface: #f8f9fb; --border: #e3e6ea;
    --text: #16191d; --muted: #5b6470; --accent: #2f5bd7;
    --success: #1f7a4d; --danger: #c0342b;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 32px 24px; background: var(--bg); color: var(--text);
    font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  }
  .wrap { max-width: 860px; margin: 0 auto; }
  h1 { font-size: 22px; margin: 0 0 4px; }
  .sub { color: var(--muted); font-size: 14px; margin: 0 0 24px; }
  .card {
    border: 1px solid var(--border); border-radius: 8px;
    padding: 20px; margin-bottom: 16px;
  }
  .meta { display: flex; gap: 26px; flex-wrap: wrap; margin-bottom: 20px; }
  .meta div { font-size: 13px; }
  .meta b { display: block; font-size: 15px; font-weight: 600; }
  .meta span { color: var(--muted); }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th {
    text-align: left; font-size: 12px; text-transform: uppercase;
    letter-spacing: 0.04em; color: var(--muted);
    padding: 8px 10px; border-bottom: 1px solid var(--border);
  }
  td { padding: 10px; border-bottom: 1px solid var(--border); }
  tr:last-child td { border-bottom: none; }
  code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 13px; background: var(--surface);
    padding: 1px 5px; border-radius: 4px;
  }
  .flag {
    display: inline-block; padding: 2px 9px; border-radius: 999px;
    font-size: 12px; font-weight: 650; border: 1px solid var(--border);
  }
  .flag.on { color: var(--success); background: #f0f8f3; border-color: #bfe0cd; }
  .flag.off { color: var(--muted); background: var(--surface); }
  .live {
    display: inline-flex; align-items: center; gap: 7px;
    font-size: 13px; color: var(--muted);
  }
  .dot {
    width: 8px; height: 8px; border-radius: 50%; background: var(--success);
    animation: pulse 2s ease-in-out infinite;
  }
  @keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.3; } }
  .log {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 12.5px; color: var(--muted);
    max-height: 190px; overflow-y: auto;
  }
  .log div { padding: 3px 0; border-bottom: 1px solid var(--surface); }
  .log b { color: var(--text); font-weight: 600; }
  .note {
    background: #fffdf5; border: 1px solid #f0e2b0;
    border-radius: 8px; padding: 14px 16px; font-size: 13.5px;
  }
  .note b { display: block; margin-bottom: 4px; }
</style>
</head>
<body>
<div class="wrap">
  <h1>Feature Flag Demo</h1>
  <p class="sub">This page reads flags from the SDK. Toggle a flag in the dashboard and watch it change here, with no reload.</p>

  <div class="card">
    <div class="meta">
      <div><span>Environment</span><b id="env">-</b></div>
      <div><span>Version</span><b id="version">-</b></div>
      <div><span>Connected</span><b id="status">-</b></div>
    </div>
    <div id="flags"></div>
  </div>

  <div class="card">
    <div class="live" style="margin-bottom:12px">
      <span class="dot"></span> Live updates
    </div>
    <div class="log" id="log"></div>
  </div>

  <div class="card note">
    <b>How this works</b>
    The server holds the SDK's in-memory snapshot. <code>isEnabled()</code> reads
    that snapshot directly, so evaluating a flag never touches the network.
    The SSE stream only signals that something changed; the SDK then fetches
    the new snapshot. Flip a flag in the dashboard at
    <code>http://localhost:3000</code> and it appears here within a second.
  </div>
</div>

<script>
  // Poll the server for the rendered state. The page itself is static;
  // only the data changes.
  async function refresh() {
    try {
      const res = await fetch("/api/state");
      const state = await res.json();
      document.getElementById("env").textContent = state.environment;
      document.getElementById("version").textContent = state.version;
      document.getElementById("status").textContent = state.status;
      document.getElementById("flags").innerHTML = state.flagsHtml;
    } catch (err) {
      document.getElementById("status").textContent = "error";
    }
  }
  refresh();
  setInterval(refresh, 700);
</script>
</body>
</html>`;

const server = createServer(async (req, res) => {
  if (req.url === "/api/state") {
    const snapshot = client.getSnapshot();
    const body = JSON.stringify({
      environment: snapshot?.environment ?? "-",
      version: client.getVersion() ?? "-",
      status: client.getStatus(),
      flagsHtml: renderFlags(),
      log: recentLog,
    });
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(body);
    return;
  }

  res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
  res.end(PAGE);
});

// A short log of recent activity, shown on the page.
let recentLog = [];

client.on("status", (detail) => {
  recentLog.unshift(`<div><b>status</b> ${detail.status}</div>`);
  recentLog = recentLog.slice(0, 20);
});

client.on("change", (next) => {
  const enabled = Object.entries(next.flags)
    .filter(([, value]) => value)
    .map(([key]) => key)
    .join(", ");
  recentLog.unshift(
    `<div><b>change</b> version ${next.version}` +
      (enabled ? ` &mdash; on: ${escapeHtml(enabled)}` : "") +
      `</div>`,
  );
  recentLog = recentLog.slice(0, 20);

  console.log(
    `change: version ${next.version}` +
      (enabled ? ` on: ${enabled}` : " (no flags on)"),
  );
});

server.listen(port, () => {
  console.log(`demo app running at http://localhost:${port}`);
  console.log(`watching ${snapshot.environment} for changes...`);
});

async function shutdown() {
  console.log("\nshutting down");
  await client.close();
  server.close(() => process.exit(0));
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
