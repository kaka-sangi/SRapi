# SRapi web console (Next.js). Build context MUST be the repository root, because
# this app imports the generated TypeScript SDK from ../../packages/sdk by
# relative path and Turbopack's workspace root is pinned to the repo root.
#
#   docker build -f apps/web/Dockerfile -t srapi-web .
#
FROM node:22-bookworm-slim AS build
WORKDIR /repo
ENV NEXT_TELEMETRY_DISABLED=1
# Install the web app's own dependencies (it has its own lockfile).
COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN cd apps/web && npm ci
# Source for the app plus the SDK it imports at build time.
COPY apps/web ./apps/web
COPY packages ./packages
RUN cd apps/web && npm run build

FROM node:22-bookworm-slim AS runtime
WORKDIR /repo/apps/web
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    # Same-origin proxy target for /api and /v1 (override per deployment).
    SRAPI_API_PROXY_TARGET=http://api:8080
# Carry over the built app, its node_modules, and the SDK it resolves at runtime.
COPY --from=build /repo/apps/web ./
COPY --from=build /repo/packages /repo/packages
EXPOSE 3000
USER node
CMD ["npm", "run", "start"]
