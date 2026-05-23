#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${SRAPI_BASE_URL:-http://127.0.0.1:8080}"
API_KEY="${SRAPI_API_KEY:?SRAPI_API_KEY is required}"
MODEL="${SRAPI_MODEL:-gpt-4o-mini}"
GEMINI_MODEL="${SRAPI_GEMINI_MODEL:-$MODEL}"

json_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"

  if [[ -n "$body" ]]; then
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer ${API_KEY}" \
      -H "Content-Type: application/json" \
      -H "Accept: application/json" \
      --data "$body" \
      "${BASE_URL}${path}"
  else
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer ${API_KEY}" \
      -H "Accept: application/json" \
      "${BASE_URL}${path}"
  fi
}

echo "GET /v1/models"
json_request GET "/v1/models"
echo

echo "POST /v1/chat/completions"
json_request POST "/v1/chat/completions" "$(printf '{"model":"%s","messages":[{"role":"user","content":"hello from curl chat"}],"stream":false}' "$MODEL")"
echo

echo "POST /v1/responses"
json_request POST "/v1/responses" "$(printf '{"model":"%s","input":"hello from curl responses","stream":false}' "$MODEL")"
echo

echo "POST /v1/messages"
json_request POST "/v1/messages" "$(printf '{"model":"%s","max_tokens":128,"messages":[{"role":"user","content":"hello from curl messages"}],"stream":false}' "$MODEL")"
echo

echo "GET /v1beta/models"
json_request GET "/v1beta/models"
echo

echo "POST /v1beta/models/{model}:countTokens"
json_request POST "/v1beta/models/${GEMINI_MODEL}:countTokens" '{"contents":[{"role":"user","parts":[{"text":"count this Gemini-compatible request"}]}]}'
echo

echo "POST /v1/messages/count_tokens"
json_request POST "/v1/messages/count_tokens" "$(printf '{"model":"%s","messages":[{"role":"user","content":"count this Anthropic-compatible request"}]}' "$MODEL")"
echo

if [[ -n "${SRAPI_ADMIN_SESSION:-}" ]]; then
  echo "GET /api/v1/admin/ops/realtime/slots"
  curl -fsS \
    -H "Accept: application/json" \
    -H "Cookie: srapi_session=${SRAPI_ADMIN_SESSION}" \
    "${BASE_URL}/api/v1/admin/ops/realtime/slots?page=1&page_size=20"
  echo
else
  echo "Skip /api/v1/admin/ops/realtime/slots: SRAPI_ADMIN_SESSION is not set."
fi
