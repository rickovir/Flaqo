// Compiles the README's SDK example so it cannot drift.
//
// It is type checked with the package's own tsconfig, but it is not run:
// it would need a live API.

import { createFeatureFlagClient } from "../src/index.js";

async function readmeExample(): Promise<void> {
  const client = createFeatureFlagClient({
    apiUrl: "https://api.example.com",
    sdkKey: "ff_client_...",
  });

  // Fetches the first snapshot before resolving.
  await client.initialize();

  // Reads local memory. This never performs a network request.
  const enabled = client.isEnabled("new_dashboard", false);
  void enabled;

  // Called after a new snapshot is applied.
  const unsubscribe = client.on("change", (snapshot) => {
    console.log("configuration is now at version", snapshot.version);
  });

  // Stops the stream and releases resources.
  unsubscribe();
  await client.close();
}

void readmeExample;
