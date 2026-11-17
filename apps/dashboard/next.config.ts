import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // The standalone output is a self contained server with only the
  // modules it needs, which keeps the runtime image small.
  output: "standalone",
  // The dashboard talks to the API through the browser, so the API base
  // URL must be reachable from the browser. It is a public value and
  // contains no secret.
  env: {
    NEXT_PUBLIC_API_BASE_URL:
      process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080",
  },
};

export default nextConfig;
