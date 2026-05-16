---
name: pitch
description: >
  Share a report, proposal, or insight as a clean rendered page via pitchbin.
  Creates a short shareable URL. Use when the user says "pitch this", "share this",
  "make a page", or invokes /pitch.
---

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
   - A brief summary of what will be shared
   - Ask them to confirm, edit the title, or adjust the content

   Example question: "What title should this pitch have?" with options based on the content.

4. **Submit via CLI.** Once confirmed, pipe the markdown into the pitchbin CLI:

   ```bash
   cat <<'PITCH_EOF' | npx pitchbin --title "The Title" --author "claude" -
   # Your Markdown Here
   ...
   PITCH_EOF
   ```

   The CLI computes proof-of-work locally and submits. It prints the URL to stdout.

   If `PITCHBIN_URL` env var is set, it uses that. Otherwise defaults to `https://pitchbin.io`.

5. **Return the URL.** Show the user the pitchbin URL so they can share it.

## Important

- Always let the user review and confirm before submitting
- The CLI handles PoW computation — no server round-trip needed for the challenge
- Content is public — warn the user if the content appears to contain secrets or credentials
- Pitches are permanent by default. Only pass `--expires 7d|30d|90d` if the user explicitly asks for expiry
