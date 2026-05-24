<!-- last-verified: 2026-05-24 | 8f7875e -->

# pitchbin

Markdown → clean shareable page. One API call. No auth.

```
npx pitchbin --title "Q3 Migration Plan" MIGRATION.md
→ https://pitchbin.xyz/q3-migration-plan
```

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

Single POST to `/api/pitch` with a PoW stamp. Check `GET /api/info` for difficulty. Edit and delete via `PUT`/`DELETE /api/pitch/{id}` with the secret returned at creation. No auth headers, no setup.

## Inline review

Viewers highlight any text and leave a comment. Annotations appear in a sidebar, linked to the exact passage. It's Google Docs comments for a page that took one second to create.

## Self-host

```
docker compose -f docker/compose.yaml up
```

Single Go binary, SQLite, zero external dependencies. 15MB, runs on anything. Tunables via flags (`-base-url`, `-pow-bits`, `-rate-limit`, `-max-size`, `-trusted-proxy`) — run the binary with `-h` for the full list.

## CLI

```
npx pitchbin [options] <file|->
npx pitchbin --update ID --secret SECRET <file|->
npx pitchbin --revise ID --secret SECRET <file|->
npx pitchbin --delete ID --secret SECRET

  --url URL        Pitchbin server URL (or PITCHBIN_URL env)
  --title TEXT     Pitch title (default: parsed from first # heading)
  --author TEXT    Author name
  --expires SPEC   7d, 30d, 90d, permanent (default: 7d)
  --slug TEXT      URL slug (default: derived from title)
  --private        Add random suffix to URL (unguessable)
  --bits N         PoW difficulty override (default: auto from server)
  --update ID      Overwrite an existing pitch
  --revise ID      Add a new revision (keeps history)
  --delete ID      Delete an existing pitch
  --secret SECRET  Edit secret (returned on creation)
  -                Read markdown from stdin
```

Submission prints the URL and an edit secret. Save the secret if you want to update or delete the pitch later.

## License

MIT
