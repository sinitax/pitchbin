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

3. **Judge visibility and lifetime.** Before confirming, classify the content:
   - **Draft or internal-use signals** — work-in-progress, notes-to-self, "internal", "for the team", "don't share yet", unfinished sections, raw analysis, anything not obviously meant for outside eyes. When these are present, **default to `--private`** (adds an unguessable random suffix to the URL) and do not ask — just note in the confirmation that you've made it private because it looks like a draft/internal doc.
   - **Public-ready signals** — a polished, self-contained report/proposal clearly written for an external or wide audience. Only in this case ask the user whether it should be public or private (default to private if they don't care).
   - **Lifetime:** prefer a limited-time pitch. Default to 7 days; choose `--expires 30d` or `90d` only when the content clearly has lasting value. **Avoid `--expires permanent`** unless the user explicitly asks for a permanent link.

4. **Confirm with the user.** Use `AskUserQuestion` to show the user:
   - The proposed **title**
   - A suggested **slug** (the URL path, e.g. `q3-migration-plan` for `pitchbin.xyz/q3-migration-plan`). Derive it from the title — lowercase, hyphens, short and memorable.
   - The chosen **visibility** (private/public) and **expiry**, with a one-line reason
   - A brief summary of what will be shared
   - Ask them to confirm, edit the title/slug, or adjust the content
   - For a public-ready document, include the public-vs-private choice here per step 3

5. **Submit via CLI.** Once confirmed, pipe the markdown into the pitchbin CLI. Add `--private` and the chosen `--expires` per step 3:

   ```bash
   cat <<'PITCH_EOF' | npx pitchbin --title "The Title" --slug "the-slug" --author "claude" --private -
   # Your Markdown Here
   ...
   PITCH_EOF
   ```

   The CLI computes proof-of-work locally and submits. It prints the URL, secret, and expiry to stdout.

   If `PITCHBIN_URL` env var is set, it uses that. Otherwise defaults to `https://pitchbin.xyz`.

6. **Save the secret.** The CLI returns a `secret:` value. **You must remember this** — it's required to update or revise the pitch later. Tell the user the secret so they have it too.

7. **Return the URL.** Show the user the pitchbin URL so they can share it.

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
- Default to `--private` for anything that looks like a draft or internal doc; only ask public-vs-private when the document looks public-ready (see step 3)
- A public (non-private) URL is guessable — warn the user if the content appears to contain secrets or credentials
- Pitches expire after 7 days by default. Prefer a limited lifetime; pass `--expires 30d|90d` for lasting content, and avoid `--expires permanent` unless the user explicitly asks
- Save the edit secret — without it the pitch cannot be updated
