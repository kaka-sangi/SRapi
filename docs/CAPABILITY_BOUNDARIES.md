# Capability Boundaries

This document records intentional SRapi capability boundaries that must be
visible to operators and future implementers. These are not hidden fallbacks:
configuration outside these boundaries must be rejected or routed only to an
upstream that natively supports it.

| Area | Current Supported Boundary | Explicit Non-Support | Trigger / Next Action |
| --- | --- | --- | --- |
| Reverse proxy fingerprinting | Preset uTLS HTTP/1.1 ClientHello over direct, HTTP CONNECT, SOCKS5, or SOCKS5H HTTPS/WSS egress | Field-by-field ClientHelloSpec, JA3/JA4 matching, HTTP/2 SETTINGS/Akamai fingerprint templates | Product asks for full fingerprint emulation; implement as a dedicated fingerprint-profile batch |
| Gateway web search | Provider-hosted passthrough requiring upstream `web_search.v1` | Server-side Tavily/Brave/Copilot fulfillment for gateway requests | Product decides SRapi should fulfill search for non-hosted upstreams |
| Account scheduling state | Hot scheduler state in `provider_accounts.metadata_json` | Indexed typed scheduler-state columns today | Active provider accounts exceed 5,000 |

## Reverse Proxy Fingerprints

Current runtime support is limited to preset uTLS ClientHello templates for
HTTP/1.1 HTTPS/WSS egress. Supported proxy combinations are direct egress,
HTTP CONNECT, and SOCKS5/SOCKS5H tunnels. The runtime does not expose a
field-by-field `ClientHelloSpec`, custom cipher/curve/extension ordering,
GREASE control, JA3/JA4 snapshot matching, or Akamai-style HTTP/2 fingerprint
templates.

Unsupported HTTP/2 fingerprint requirements such as `require_h2` and custom
HTTP/2 SETTINGS policies must continue to return `unsupported_egress_profile`
instead of silently falling back to a different network identity.

## Gateway Web Search

Gateway web search is provider-hosted passthrough only. Responses
`web_search` / `web_search_preview` and Anthropic hosted web-search tools are
preserved as hosted tool declarations and require `web_search.v1` capability on
the selected upstream.

SRapi does not server-side fulfill gateway `web_search` with Tavily, Brave, or
Copilot search functions when the chosen upstream lacks hosted search. Admin
Copilot search remains a separate admin feature, not a gateway fallback.

## Account Scheduling State

Provider account scheduling state currently lives in `provider_accounts.metadata_json`.
Hot keys include `rate_limited_at`, `overload_until`, `schedulable`, and
expiration-style metadata such as `expires_at`. This is acceptable for current
account-pool sizes because the scheduler already materializes candidates in
process.

Scale trigger: when active provider accounts exceed 5,000, promote hot
scheduling-state keys into typed indexed columns on `provider_accounts`, with a
migration and scheduler candidate-query tests. The intended schema touch point
is `apps/api/ent/schema/provideraccount.go`.
