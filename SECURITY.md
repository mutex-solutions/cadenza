# Security

## Trust model

- **One owner.** The owner logs in with a password (argon2id). Web sessions are random tokens stored as SHA-256
  hashes, sent as an HttpOnly, SameSite=Lax cookie (or a Bearer token from the Android app).
- **Machines** are paired once with a short code the owner types into the app. A machine key can only create
  session keys for that machine. Removing a machine stops its keys at once.
- **Agent sessions** each get their own key, tied to the agent's session id. A session key can show, ask and read
  replies for threads on its own machine only. Keys expire after 7 days unused and can be revoked one by one.
- Agents running as the same system user on one machine can read each other's keys. Use separate users when that
  matters.
- **Files** never pass through the server. It signs short-lived upload and download links for S3-compatible
  storage. Uploads carry a signed SHA-256 checksum, so storage rejects bytes that don't match.

## What an agent can show

- Template data is validated against fixed blocks. Markdown is rendered on the server with raw HTML dropped and
  only safe link schemes kept.
- A web page an agent shows is untrusted. The apps run it in a sandboxed iframe (scripts allowed, no same-origin)
  with a Content-Security-Policy that blocks network requests. **Known limit:** browsers do not yet let a page block
  WebRTC, so a malicious page could still send out what it contains and your IP address. It cannot reach your
  login, the app or other posts.
- The CLI refuses `.env` and key files and hides secret-looking text (API keys, tokens, private keys) before upload.

## Notifications

Notifications carry the agent, the machine and the agent's short title, never the work itself. Android
notifications go through Expo or Firebase using the self-hoster's own credentials.

## Reporting a vulnerability

Report vulnerabilities privately through
[GitHub security advisories](https://github.com/mutex-solutions/cadenza/security/advisories/new).
Do not open a public issue. Include a description, steps to reproduce, and the impact.
