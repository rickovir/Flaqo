// Live check for the SDK's SSE update path.
//
// The SDK connects, the test toggles a flag through the dashboard API,
// and the SDK must observe the change without a page reload.
//
// Run with:
//
//   node test/live-stream.mjs <apiUrl> <sdkKey> <apiBase> <csrfToken> <projectId> <flagId> <envId>
import { createFeatureFlagClient } from "../dist/index.js";

const [apiUrl, sdkKey, apiBase, csrfToken, projectId, flagId, envId, cookie, flagKey] =
  process.argv.slice(2);

if (!envId) {
  console.error(
    "usage: node test/live-stream.mjs <apiUrl> <sdkKey> <apiBase> <csrf> <projectId> <flagId> <envId> [cookie]",
  );
  process.exit(1);
}

const client = createFeatureFlagClient({ apiUrl, sdkKey });

let failures = 0;
function check(name, condition, detail = "") {
  if (!condition) {
    failures += 1;
  }
  console.log(`  ${condition ? "PASS" : "FAIL"}  ${name}${detail ? ` (${detail})` : ""}`);
}

await client.initialize();
const initialVersion = client.getVersion() ?? 0;
console.log(`initial version: ${initialVersion}`);

const versions = [];
client.on("change", (snapshot) => {
  versions.push(snapshot.version);
});

// Wait for the stream to connect.
const deadline = Date.now() + 10_000;
while (client.getStatus() !== "connected" && Date.now() < deadline) {
  await new Promise((r) => setTimeout(r, 100));
}
check("stream connected", client.getStatus() === "connected", client.getStatus());

console.log("\ntoggling the flag through the dashboard API");
const toggle = await fetch(
  `${apiBase}/api/v1/flags/${flagId}/environments/${envId}`,
  {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
      // The dashboard API authenticates with the session cookie.
      Cookie: cookie ?? "",
    },
    body: JSON.stringify({ enabled: true }),
  },
);
check("toggle accepted", toggle.ok, `status=${toggle.status}`);

// Wait for the SDK to observe the change.
const changeDeadline = Date.now() + 10_000;
while (versions.length === 0 && Date.now() < changeDeadline) {
  await new Promise((r) => setTimeout(r, 50));
}

check("change event received", versions.length > 0, `versions=${versions.join(",")}`);
check("version advanced", (client.getVersion() ?? 0) > initialVersion,
  `${initialVersion} -> ${client.getVersion()}`);
check(
  "flag now enabled",
  client.isEnabled(flagKey ?? "stream_demo") === true,
  `flag=${flagKey ?? "stream_demo"}`,
);

await client.close();

console.log(
  failures === 0
    ? "\nAll live stream checks passed."
    : `\n${failures} live check(s) failed.`,
);
process.exit(failures === 0 ? 0 : 1);
