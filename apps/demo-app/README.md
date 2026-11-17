# Demo application

A minimal application that uses the feature flag SDK. It shows the
current flag values and updates live when a flag changes in the
dashboard, with no redeploy and no page reload.

It exists to demonstrate the SDK's behaviour, not as a product feature.

## Run it

```bash
npm install
npm run build --prefix ../../packages/sdk   # the SDK must be built first

SDK_KEY=ff_client_... npm start
```

Then open http://localhost:4000.

`SDK_KEY` is a key issued from the dashboard, scoped to one environment.
The key is confidential for a server key, so it is passed through the
environment rather than committed.

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `SDK_KEY` | required | The scoped SDK key |
| `API_URL` | `http://localhost:8080` | The API base URL |
| `PORT` | `4000` | The port the demo listens on |

## What it demonstrates

The server holds the SDK's in-memory snapshot. `isEnabled()` reads that
snapshot directly, so evaluating a flag never performs a network
request.

The SSE stream only signals that the configuration changed. The SDK
then fetches the new snapshot and replaces the old one atomically. The
page polls the server for the current state, which keeps the browser
side trivial while still showing the effect immediately.

## Try it

1. Open http://localhost:3000 and sign in.
2. Open the project that holds the flag, and toggle it.
3. Watch http://localhost:4000 update within a second.

The version number in the demo app changes with the toggle, and the
server log records each change:

``` text
change: version 4 on: demo_flag
```
