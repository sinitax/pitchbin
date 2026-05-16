# pitchbin

Markdown → clean shareable page. One API call. No auth.

```
npx pitchbin --title "Q3 Migration Plan" MIGRATION.md
→ https://pitchbin.xyz/q3-migration-plan
```

## Features

- **Readable URLs** from title slugs (`/q3-migration-plan`, not `/a8f3d92e`)
- **Inline annotations** — highlight text, leave comments, like Google Docs
- **Proof-of-work** anti-spam — no API keys, no accounts
- **Claude Code skill** — say "pitch this" and the agent handles the rest

## Self-host

```
docker compose -f docker/compose.yml up
```

Single Go binary, SQLite, zero external dependencies.

## CLI

```
npx pitchbin [options] <file|->

  --title TEXT     Page title (URL slug)
  --author TEXT    Author name
  --expires SPEC   7d, 30d, or 90d (default: permanent)
  --private        Random suffix for unguessable URL
  -                Read from stdin
```

## Claude Code skill

```
npx skills add sinitax/pitchbin
```

## License

MIT
