Your agent just finished a 20-minute deep dive — architecture review, cost analysis, migration plan, the works. Now it needs to *share* that with someone who isn't sitting in a terminal.

**pitchbin** turns markdown into a clean, shareable page. One API call. One short URL. No account, no login, no viewer setup.

## How it works

```
echo '# My Proposal ...' | npx pitchbin --title "Q3 Migration Plan"
→ https://pitchbin.xyz/q3-migration-plan
```

That's it. The URL is live, rendered, and ready to send to anyone.

## Why not just use a Gist?

| | Gist | pitchbin |
|---|---|---|
| URL | `gist.github.com/user/a8f3...d92e` | `pitchbin.xyz/q3-plan` |
| Auth required | GitHub token | None (proof-of-work) |
| Viewer experience | Code hosting UI | Clean rendered page |
| Inline feedback | No | Highlight + comment |
| Agent-friendly | Needs OAuth flow | Single POST |

## Built for agents

No API keys to manage. Anti-spam is handled by proof-of-work — your agent computes a SHA-256 partial collision locally in under a second, submits it with the pitch. No round-trip, no tokens, no rate limit dance.

Install the Claude Code skill and your agent can pitch directly from a conversation:

```
npx skills add sinitax/pitchbin
```

Then just say "pitch this" and the agent handles the rest — drafts the page, confirms with you, computes PoW, posts it, hands you the link.

## Inline review

Viewers can highlight any text and leave a comment. Annotations appear in a sidebar, linked to the exact passage. Set your name once and it sticks across sessions. It's like Google Docs comments but for a page that took one second to create.

## Self-host in 30 seconds

```
docker compose -f docker/compose.yml up
```

Single Go binary, SQLite storage, zero external dependencies. Or deploy the binary directly — it's 15MB and runs on anything.

## Open source

MIT licensed. The whole thing is under 2000 lines of Go. No framework, no build step, no JS bundler. The rendered page is server-side HTML with inline CSS.

[GitHub](https://github.com/sinitax/pitchbin) · [Install CLI](https://www.npmjs.com/package/pitchbin) · [Install Skill](https://github.com/sinitax/pitchbin)
