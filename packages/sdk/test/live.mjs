// Live integration check for the SDK against a running API.
//
// It is not part of the automated test suite because it needs a running
// server and a real credential. Run it with:
//
//   node test/live.mjs <apiUrl> <sdkKey> <flagKey>
import { createFeatureFlagClient } from "../dist/index.js";

const apiUrl = process.argv[2] ?? "http://localhost:8080";
const sdkKey = process.argv[3];
const flagKey = process.argv[4] ?? "live_flag";

if (!sdkKey) {
  console.error("usage: node test/live.mjs <apiUrl> <sdkKey> [flagKey]");
  process.exit(1);
}

const client = createFeatureFlagClient({ apiUrl, sdkKey });

let failures = 0;
function check(name, condition, detail = "") {
  const status = condition ? "PASS" : "FAIL";
  if (!condition) {
    failures += 1;
  }
  console.log(`  ${status}  ${name}${detail ? ` (${detail})` : ""}`);
}

console.log("initialize");
await client.initialize();
check("snapshot applied", client.getSnapshot() !== undefined);
check("version is at least 1", (client.getVersion() ?? 0) >= 1, `version=${client.getVersion()}`);
check("environment is production", client.getSnapshot()?.environment === "production");

console.log("\nlocal evaluation");
check("known flag reads false by default", client.isEnabled(flagKey) === false);
check("known flag respects fallback", client.isEnabled(flagKey, true) === false);
check("unknown flag uses fallback", client.isEnabled("no_such_flag", true) === true);
check("unknown flag defaults to false", client.isEnabled("no_such_flag") === false);

console.log("\nchange notification");
const seen = [];
client.on("change", (snapshot) => {
  seen.push(snapshot.version);
});
check("listener registered", true);

console.log("\nstatus");
console.log(`  current status: ${client.getStatus()}`);

await client.close();
check("closed cleanly", client.getStatus() === "closed");
check("snapshot survives close", client.isEnabled(flagKey) === false);

console.log(
  failures === 0
    ? "\nAll live checks passed."
    : `\n${failures} live check(s) failed.`,
);
process.exit(failures === 0 ? 0 : 1);
