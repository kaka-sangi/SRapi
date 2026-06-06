# Security Policy

## Supported versions

SRapi is pre-1.0 and under active development. Security fixes target the `main` branch and the latest
tagged release. Run a recent build; older snapshots are not separately patched.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately through GitHub's [security advisories](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
("Report a vulnerability" on the repository's *Security* tab), or contact the maintainers directly.

When reporting, include:

- A description of the issue and its impact.
- Steps to reproduce (a proof of concept if possible).
- Affected component (gateway, control plane, reverse-proxy runtime, payments, auth, …) and version
  or commit.

We aim to acknowledge reports promptly and will coordinate a fix and disclosure timeline with you.
Please give us a reasonable window to remediate before any public disclosure.

## Security posture

SRapi is designed to be self-hosted and to keep operator and end-user secrets under the operator's
control. Key properties (see [`docs/SECURITY_MODEL.md`](docs/SECURITY_MODEL.md) for the full model):

- **Credentials are write-only.** Provider credentials and other secrets are encrypted at rest and
  are never returned by the API or shown in the console after creation.
- **Deployment secrets stay in the environment.** Values such as SMTP passwords, OAuth client
  secrets, and CAPTCHA secrets live in deployment environment variables, not in admin settings, and
  are never serialized into API responses or audit logs.
- **Production refuses weak configuration.** Outside `SERVER_MODE=local`, the server will not boot
  with weak or default secrets or a default admin password.
- **API keys are stored hashed** (HMAC with a server pepper); plaintext is shown once at creation.
- **Console sessions** use HttpOnly cookies with CSRF protection; TOTP 2FA is available.
- **Outbound SSRF guard.** Direct reverse-proxy upstream/refresh dials screen the resolved remote IP
  (loopback, RFC1918/ULA, link-local + cloud metadata, CGNAT, multicast) in non-local mode.
- **Audit evidence** for administrative writes is recorded without leaking secrets.

## Compliance boundary (reverse-proxy runtime)

SRapi's reverse-proxy runtime provides only self-hosted runtime capabilities and isolation
mechanisms. It does **not** include any upstream Terms-of-Service bypass, CAPTCHA solving, cookie
scraping, or token-harvesting logic. Operators are responsible for ensuring that the accounts,
regions, network egress, and automation they configure comply with each upstream provider's terms,
and they bear the associated compliance and account-ban risk.
