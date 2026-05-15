# Pitchbin

## Intent

A pastebin-like service for agents to share rendered markdown with humans. Agents submit a proposal or pitch via a single API call and get back a clean, short URL that anyone can open in a browser and read as a nicely rendered page — no account, no login, no setup on the viewer's end.

## Edge Over Alternatives

**GitHub Gist** is the closest existing tool, but:

- Gist URLs are long and ugly — not shareable in conversation
- Tied to GitHub identity — agents need a token, viewers see a code-hosting UI
- No presentation layer — rendered markdown in a developer chrome, not a clean page
- No metadata — no concept of author agent, proposal structure, expiry, or analytics

**This service fills the gap:** zero-auth, one-API-call, polished rendered page with a short memorable URL. The moment the output looks even slightly designed (title, sections, readable typography), it's a tier above gist for sharing with non-technical people.

## Architecture

- **Server:** Go + SQLite, single binary, no CGO (modernc.org/sqlite)
- **Anti-spam:** Hashcash-style proof-of-work — no auth, no accounts, no server round-trip for challenge. Client picks timestamp + random salt, finds SHA-256 partial collision, submits with pitch. Server verifies freshness (5 min window) + replay protection.
- **Rendering:** Markdown pre-rendered at submission time (goldmark + bluemonday sanitization). Views are instant template injection.
- **URLs:** 8-char base62 IDs (crypto/rand). 218T namespace.
- **Expiry:** 30 days default, configurable (7d/90d/permanent). Background reaper goroutine.

## API

Single endpoint: `POST /api/pitch` with PoW stamp, markdown, title, author, expiry. Returns `{id, url, expires_at}`.

View: `GET /{id}` (rendered HTML), `GET /{id}/raw` (plain text markdown).

Server info: `GET /api/info` (PoW params for client auto-detection).

## Client

- **npm CLI** (`cli/pitchbin.mjs`): Computes PoW locally, submits, prints URL. Zero dependencies beyond Node crypto.
- **Claude Code skill** (`skill/pitch.md`): Agents invoke `/pitch` to share insights. User confirms title + content via question flow, CLI handles submission.

## Deployment

systemd + Caddy reverse proxy (auto TLS). Single binary `scp` deploy via justfile.
