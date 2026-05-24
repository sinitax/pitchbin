---
name: pitch
description: >
  Share a report, proposal, or insight as a clean rendered page via pitchbin.
  Creates a short shareable URL. Use when the user says "pitch this", "share this",
  "make a page", or invokes /pitch.
---
<!-- last-verified: 2026-05-24 | 8f7875e -->

# Pitch to Pitchbin

You are helping the user share content via pitchbin — a service that renders markdown as a clean, shareable web page.

## Steps

1. **Identify the content.** Look at the recent conversation context. Identify what the user likely wants to share — a report, analysis, proposal, insight, or summary. If unclear, ask.

2. **Draft the pitch.** Write the content as clean, well-structured markdown. Include:
   - A clear title
   - Well-organized sections with headers
   - Tables, code blocks, or lists where appropriate
   - Keep it focused and readable for the intended audience

3. **Confirm with the user.** Use `AskUserQuestion` to show the user:
   - The proposed **title**
   - A suggested **slug** (the URL path, e.g. `q3-migration-plan` for `pitchbin.xyz/q3-migration-plan`). Derive it from the title — lowercase, hyphens, short and memorable.
   - A brief summary of what will be shared
   - Ask them to confirm, edit the title/slug, or adjust the content

4. **Submit via CLI.** Once confirmed, pipe the markdown into the pitchbin CLI:

   ```bash
   cat <<'PITCH_EOF' | npx pitchbin --title "The Title" --slug "the-slug" --author "claude" -
   # Your Markdown Here
   ...
   PITCH_EOF
   ```

   The CLI computes proof-of-work locally and submits. It prints the URL, secret, and expiry to stdout.

   If `PITCHBIN_URL` env var is set, it uses that. Otherwise defaults to `https://pitchbin.xyz`.

5. **Save the secret.** The CLI returns a `secret:` value. **You must remember this** — it's required to update or revise the pitch later. Tell the user the secret so they have it too.

6. **Return the URL.** Show the user the pitchbin URL so they can share it.

## Updating & Revisions

If the user wants to edit a pitch that was already published:

- **Update (overwrite):** Replace the content entirely. Previous version is lost.
  ```bash
  cat <<'PITCH_EOF' | npx pitchbin --update "pitch-id" --secret "THE_SECRET" -
  # Updated content
  PITCH_EOF
  ```

- **Revise (with history):** Add a new revision. Previous versions remain accessible via the revision picker.
  ```bash
  cat <<'PITCH_EOF' | npx pitchbin --revise "pitch-id" --secret "THE_SECRET" -
  # Revised content
  PITCH_EOF
  ```

- **Delete:** Remove the pitch entirely. The URL stops resolving.
  ```bash
  npx pitchbin --delete "pitch-id" --secret "THE_SECRET"
  ```
  Confirm with the user before deleting — this cannot be undone.

The pitch ID is the URL slug (e.g. `q3-migration-plan` from `pitchbin.xyz/q3-migration-plan`).

## Important

- Always let the user review and confirm before submitting
- The CLI handles PoW computation — no server round-trip needed for the challenge
- Content is public — warn the user if the content appears to contain secrets or credentials
- Pitches expire after 7 days by default. Pass `--expires 30d|90d|permanent` to change
- Save the edit secret — without it the pitch cannot be updated
