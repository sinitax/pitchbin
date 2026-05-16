# pitchbin

Markdown → clean shareable page. One API call. No auth.

```
npx pitchbin --title "Q3 Migration Plan" MIGRATION.md
```

> https://pitchbin.xyz/q3-migration-plan

That's it. The URL is live, rendered, and ready to send to anyone.

## Built for agents

Your agent just finished a deep dive — architecture review, cost analysis, migration plan. Now it needs to *share* that with someone who isn't in a terminal.

No API keys to manage. Anti-spam is proof-of-work: compute a SHA-256 partial collision locally, submit it with the pitch. No round-trip, no tokens, no OAuth dance.

### Claude Code skill

```
npx skills add sinitax/pitchbin
```

Say "pitch this" and the agent handles the rest — drafts the page, confirms with you, computes PoW, hands you a link.

> **You:** pitch this review for the team  
> **Agent:** *(drafts, confirms, submits)*  
> https://pitchbin.xyz/auth-module-review

### Any agent

Single POST to `/api/pitch` with a PoW stamp. Check `GET /api/info` for difficulty. No auth headers, no setup.

## Inline review

Viewers highlight any text and leave a comment. Annotations appear in a sidebar, linked to the exact passage. It's Google Docs comments for a page that took one second to create.

## Self-host

```
docker compose -f docker/compose.yml up
```

Single Go binary, SQLite, zero external dependencies. 15MB, runs on anything.

## CLI

```
npx pitchbin [options] <file|->

  --title TEXT     Page title (URL slug)
  --author TEXT    Author name
  --expires SPEC   7d, 30d, or 90d (default: permanent)
  --private        Random suffix for unguessable URL
  -                Read from stdin
```

## License

MIT
