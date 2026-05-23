#!/usr/bin/env python3
import json
import os
import sys
from typing import Any

try:
    import requests
except ImportError as exc:
    raise SystemExit("Install requests first: python3 -m pip install requests") from exc


BASE_URL = os.environ.get("SRAPI_BASE_URL", "http://127.0.0.1:8080").rstrip("/")
API_KEY = os.environ.get("SRAPI_API_KEY")
MODEL = os.environ.get("SRAPI_MODEL", "gpt-4o-mini")
GEMINI_MODEL = os.environ.get("SRAPI_GEMINI_MODEL", MODEL)
ADMIN_SESSION = os.environ.get("SRAPI_ADMIN_SESSION")


def main() -> None:
    if not API_KEY:
        raise SystemExit("SRAPI_API_KEY is required")

    show("GET /v1/models", gateway("GET", "/v1/models"))

    show(
        "POST /v1/chat/completions",
        gateway(
            "POST",
            "/v1/chat/completions",
            {
                "model": MODEL,
                "messages": [{"role": "user", "content": "hello from Python chat"}],
                "stream": False,
            },
        ),
    )

    show(
        "POST /v1/responses",
        gateway(
            "POST",
            "/v1/responses",
            {
                "model": MODEL,
                "input": "hello from Python responses",
                "stream": False,
            },
        ),
    )

    show(
        "POST /v1/messages",
        gateway(
            "POST",
            "/v1/messages",
            {
                "model": MODEL,
                "max_tokens": 128,
                "messages": [{"role": "user", "content": "hello from Python messages"}],
                "stream": False,
            },
        ),
    )

    show("GET /v1beta/models", gateway("GET", "/v1beta/models"))

    show(
        "POST /v1beta/models/{model}:countTokens",
        gateway(
            "POST",
            f"/v1beta/models/{GEMINI_MODEL}:countTokens",
            {"contents": [{"role": "user", "parts": [{"text": "count this Gemini-compatible request"}]}]},
        ),
    )

    show(
        "POST /v1/messages/count_tokens",
        gateway(
            "POST",
            "/v1/messages/count_tokens",
            {
                "model": MODEL,
                "messages": [{"role": "user", "content": "count this Anthropic-compatible request"}],
            },
        ),
    )

    if ADMIN_SESSION:
        show(
            "GET /api/v1/admin/ops/realtime/slots",
            admin_get("/api/v1/admin/ops/realtime/slots?page=1&page_size=20"),
        )
    else:
        print("Skip /api/v1/admin/ops/realtime/slots: SRAPI_ADMIN_SESSION is not set.")


def gateway(method: str, path: str, body: dict[str, Any] | None = None) -> Any:
    return request(
        method,
        path,
        body,
        headers={
            "Authorization": f"Bearer {API_KEY}",
            "Accept": "application/json",
        },
    )


def admin_get(path: str) -> Any:
    return request(
        "GET",
        path,
        None,
        headers={
            "Cookie": f"srapi_session={ADMIN_SESSION}",
            "Accept": "application/json",
        },
    )


def request(method: str, path: str, body: dict[str, Any] | None, headers: dict[str, str]) -> Any:
    response = requests.request(method, f"{BASE_URL}{path}", json=body, headers=headers, timeout=30)
    if response.status_code >= 400:
        raise RuntimeError(f"{method} {path} failed: {response.status_code} {response.text}")
    return response.json()


def show(label: str, value: Any) -> None:
    print(label)
    print(json.dumps(value, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1) from error
