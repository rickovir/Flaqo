# Build the demo application.
#
# The demo depends on the SDK from the workspace, so the SDK is built
# first and copied into the image.

FROM node:24-alpine AS sdk
WORKDIR /workspace/packages/sdk

COPY packages/sdk/package.json packages/sdk/package-lock.json ./
RUN npm ci

COPY packages/sdk/ ./
RUN npm run build

FROM node:24-alpine AS app

# The demo app lives at /workspace/apps/demo-app, so the file dependency
# "../packages/sdk" in its lockfile resolves to /workspace/packages/sdk.
WORKDIR /workspace/apps/demo-app

COPY --from=sdk /workspace /workspace

COPY apps/demo-app/package.json apps/demo-app/package-lock.json ./
RUN npm install --omit=dev && npm cache clean --force

COPY apps/demo-app/server.mjs ./

USER node
EXPOSE 4000

CMD ["node", "server.mjs"]
