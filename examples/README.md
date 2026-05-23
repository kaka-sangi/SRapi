# SRapi Examples

These examples exercise SRapi's public Gateway and read-only AdminOps surfaces without embedding real upstream credentials. Configure provider accounts through the admin control plane first; client examples only send SRapi API keys or optional console session cookies.

## Environment

```bash
export SRAPI_BASE_URL="http://127.0.0.1:8080"
export SRAPI_API_KEY="<srapi gateway api key>"
export SRAPI_MODEL="gpt-4o-mini"
export SRAPI_GEMINI_MODEL="gpt-4o-mini"

# Optional, only for read-only AdminOps examples.
export SRAPI_ADMIN_SESSION="<srapi_session cookie value>"
export SRAPI_CSRF_TOKEN="<csrf token for admin writes>"
```

`SRAPI_ADMIN_SESSION` is the value of the `srapi_session` cookie, not the full `Cookie:` header. `SRAPI_CSRF_TOKEN` is listed because admin write examples need it; the realtime slot list shown here is read-only and does not require CSRF.

## Run

```bash
examples/curl/gateway.sh
```

```bash
npx --yes -p typescript@5.9.3 tsc --noEmit --strict --module ESNext --moduleResolution Bundler --target ES2022 --lib ES2022,DOM,DOM.Iterable examples/typescript/gateway.ts
```

```bash
python3 examples/python/gateway.py
```

## Covered Routes

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`
- `GET /v1beta/models`
- `POST /v1beta/models/{model}:countTokens`
- `POST /v1/messages/count_tokens`
- `GET /api/v1/admin/ops/realtime/slots`

The 2api path uses selected SRapi Provider Account OAuth/session/desktop/CLI/IDE credentials inside Provider Adapter and Reverse Proxy Runtime. Do not pass Codex, Claude Code, Antigravity, ChatGPT Web, or Gemini CLI upstream credentials from these client examples.
