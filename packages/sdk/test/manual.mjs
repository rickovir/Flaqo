import { createFeatureFlagClient } from "../dist/index.js";

// The API base URL and the SDK key come from the environment, so the
// script never contains a credential.
//
//   API_URL=http://localhost:8080 SDK_KEY=ff_client_... node test/manual.mjs
const apiUrl = process.env.API_URL ?? "http://localhost:8080";
const sdkKey = process.env.SDK_KEY;

if (!sdkKey) {
  console.error("Set SDK_KEY to an SDK key from the dashboard.");
  console.error("  API_URL=http://localhost:8080 SDK_KEY=ff_client_... node test/manual.mjs");
  process.exit(1);
}

const client = createFeatureFlagClient({ apiUrl, sdkKey });

console.log("connecting...");
await client.initialize();

const snapshot = client.getSnapshot();
console.log(`\nproject:     ${snapshot.project_id}`);
console.log(`environment: ${snapshot.environment}`);
console.log(`version:     ${snapshot.version}`);
console.log(`flags:       ${JSON.stringify(snapshot.flags)}`);

console.log("\nlocal evaluation (no network calls):");
for (const key of Object.keys(snapshot.flags)) {
  console.log(`  isEnabled("${key}")         = ${client.isEnabled(key)}`);
  console.log(`  isEnabled("${key}", true)   = ${client.isEnabled(key, true)}`);
}
console.log(`  isEnabled("not_a_flag")       = ${client.isEnabled("not_a_flag")}`);
console.log(`  isEnabled("not_a_flag", true) = ${client.isEnabled("not_a_flag", true)}`);

console.log("\nlistening for changes...");
client.on("change", (next) => {
  console.log(`\n  CHANGE  version ${next.version}`);
  console.log(`  flags:  ${JSON.stringify(next.flags)}`);
  console.log(`  isEnabled("demo_flag") = ${client.isEnabled("demo_flag")}`);
});
client.on("status", (detail) => console.log(`  status: ${detail.status}`));

console.log("\nToggle 'demo_flag' in the dashboard now. Watching for 25 seconds.\n");
await new Promise((r) => setTimeout(r, 25000));
console.log(`\nfinal version: ${client.getVersion()}`);
await client.close();
