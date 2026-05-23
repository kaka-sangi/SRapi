SRapi Console is the Next.js frontend for the SRapi management UI. The visual direction intentionally follows the existing Claude/ChatGPT-inspired editorial card style.

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

If the backend is unavailable, the console remains browsable with local demo data and labels the shell as `Demo Data`. Live sessions are labelled `Live API`.

## Checks

```bash
npm run lint
npm run build
```

## Editing

Next dev mode hot-reloads source changes. Most TSX/CSS edits update the browser automatically; config changes such as `next.config.ts` usually require restarting `npm run dev`.
