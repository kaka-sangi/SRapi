SRapi Console is the Next.js frontend for the SRapi management UI. It is a production control plane: data comes from the SRapi API and OpenAPI-generated SDK, not from local demo fixtures.

## Getting Started

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser.

The dev server proxies `/api/*` and `/v1/*` to the SRapi backend at `http://127.0.0.1:8080` by default. Override it when needed:

```bash
SRAPI_API_PROXY_TARGET=http://127.0.0.1:8080 npm run dev
```

For a remote API origin, set:

```bash
NEXT_PUBLIC_SRAPI_BASE_URL=https://your-srapi.example.com npm run dev
```

The console requires a reachable SRapi backend for login and data. If the API is unavailable, the UI reports `API offline` or shows the relevant request error; it does not fall back to local business data.

## Checks

```bash
npm run typecheck
npm run lint
npm run test
npm run build
```

The repository-level frontend gate is:

```bash
make web-check
```

Browser e2e checks require a live SRapi API. Start the backend first, then run:

```bash
make web-check-e2e
```

The e2e harness checks `/livez` and `/readyz` before building the frontend. Override the API target when needed:

```bash
SRAPI_WEB_E2E_API_URL=http://127.0.0.1:8080 make web-check-e2e
```

That target is also passed to `SRAPI_API_PROXY_TARGET`, so browser requests use the same API through the Next same-origin proxy. Set `SRAPI_WEB_E2E_DIRECT_BROWSER_API=1` only when the target API is configured for browser CORS and cookie credentials.

`npm run test:e2e:install` installs the Playwright Chromium browser only. On a fresh Linux runner that also needs OS packages, run `npm run test:e2e:install-deps` explicitly.

## Editing

Next dev mode hot-reloads source changes. Most TSX/CSS edits update the browser automatically; config changes such as `next.config.ts` usually require restarting `npm run dev`.
