#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";

const USAGE = `Usage: pitchbin [options] <file|->
       pitchbin --update ID --secret SECRET <file|->
       pitchbin --revise ID --secret SECRET <file|->
       pitchbin --delete ID --secret SECRET`

function die(msg) {
  process.stderr.write(`error: ${msg}\n`);
  process.exit(1);
}

function parseArgs() {
  const args = process.argv.slice(2);
  const opts = {
    url: process.env.PITCHBIN_URL || "https://pitchbin.xyz",
    title: "",
    author: "",
    expires: "",
    bits: 20,
    slug: "",
    private: false,
    update: "",
    revise: "",
    delete: "",
    secret: "",
    file: null,
  };

  for (let i = 0; i < args.length; i++) {
    switch (args[i]) {
      case "-h": case "--help":
        console.log(`${USAGE}

Options:
  --url URL        Pitchbin server URL (or PITCHBIN_URL env)
  --title TEXT     Pitch title (default: parsed from first # heading)
  --author TEXT    Author name
  --expires SPEC   Expiry: 7d, 30d, 90d, permanent (default: 7d)
  --slug TEXT      URL slug (default: derived from title)
  --private        Add random suffix to URL (unguessable)
  --bits N         PoW difficulty override (default: auto-detect from server)
  --update ID      Update an existing pitch by ID (overwrites)
  --revise ID      Update with revision history
  --delete ID      Delete an existing pitch by ID
  --secret SECRET  Edit secret (returned on creation)
  -                Read markdown from stdin`);
        process.exit(0);
      case "--url": opts.url = args[++i]; break;
      case "--title": opts.title = args[++i]; break;
      case "--author": opts.author = args[++i]; break;
      case "--expires": opts.expires = args[++i]; break;
      case "--bits": opts.bits = parseInt(args[++i], 10); break;
      case "--slug": opts.slug = args[++i]; break;
      case "-p": case "--private": opts.private = true; break;
      case "--update": opts.update = args[++i]; break;
      case "--revise": opts.revise = args[++i]; break;
      case "--delete": opts.delete = args[++i]; break;
      case "--secret": opts.secret = args[++i]; break;
      default:
        if (args[i].startsWith("-") && args[i] !== "-") die(`unknown flag: ${args[i]}`);
        opts.file = args[i];
    }
  }

  if (opts.delete) {
    if (!opts.secret) die("--secret is required for --delete");
    return opts;
  }

  if (!opts.file) die("missing file argument. Use - for stdin.");
  if (opts.update && !opts.secret) die("--secret is required for --update");
  if (opts.revise && !opts.secret) die("--secret is required for --revise");
  return opts;
}

function readMarkdown(file) {
  if (file === "-") return readFileSync(0, "utf-8");
  return readFileSync(file, "utf-8");
}

function parseFrontmatterTitle(markdown) {
  if (!markdown.startsWith("---\n")) return null;
  const end = markdown.indexOf("\n---", 4);
  if (end < 0) return null;
  const fm = markdown.slice(4, end);
  const match = fm.match(/^title:\s*(.+)$/m);
  if (!match) return null;
  return match[1].trim().replace(/^["']|["']$/g, "");
}

function hasLeadingZeroBits(hash, bits) {
  const fullBytes = bits >> 3;
  const remainBits = bits & 7;

  for (let i = 0; i < fullBytes; i++) {
    if (hash[i] !== 0) return false;
  }

  if (remainBits > 0) {
    const mask = 0xff << (8 - remainBits);
    if ((hash[fullBytes] & mask) !== 0) return false;
  }

  return true;
}

function computeStamp(bits) {
  const ts = Math.floor(Date.now() / 1000);
  const salt = randomBytes(8).toString("hex");
  const prefix = `pitchbin:1:${ts}:${salt}:`;

  process.stderr.write(`computing proof of work (${bits} bits)...`);
  const start = performance.now();

  for (let nonce = 0; ; nonce++) {
    const stamp = prefix + nonce;
    const hash = createHash("sha256").update(stamp).digest();
    if (hasLeadingZeroBits(hash, bits)) {
      const elapsed = ((performance.now() - start) / 1000).toFixed(1);
      process.stderr.write(` done (${nonce.toLocaleString()} hashes, ${elapsed}s)\n`);
      return stamp;
    }
  }
}

async function fetchInfo(url) {
  const resp = await fetch(`${url}/api/info`);
  if (!resp.ok) die(`failed to fetch server info: ${resp.status}`);
  return resp.json();
}

async function submit(url, stamp, markdown, title, slug, author, expires, isPrivate) {
  const payload = { stamp, markdown, title, author, expires, private: isPrivate };
  if (slug) payload.slug = slug;
  const resp = await fetch(`${url}/api/pitch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

  const body = await resp.json();
  if (!resp.ok) die(`submission failed: ${body.error || resp.status}`);
  return body;
}

async function updatePitch(url, id, secret, stamp, markdown, title, author, expires, revise) {
  const resp = await fetch(`${url}/api/pitch/${id}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      "X-Pitch-Secret": secret,
    },
    body: JSON.stringify({ stamp, markdown, title, author, expires, revise: !!revise }),
  });

  const body = await resp.json();
  if (!resp.ok) die(`update failed: ${body.error || resp.status}`);
  return body;
}

async function deletePitch(url, id, secret) {
  const resp = await fetch(`${url}/api/pitch/${id}`, {
    method: "DELETE",
    headers: { "X-Pitch-Secret": secret },
  });

  const body = await resp.json();
  if (!resp.ok) die(`delete failed: ${body.error || resp.status}`);
  return body;
}

async function main() {
  const opts = parseArgs();

  // Handle delete
  if (opts.delete) {
    await deletePitch(opts.url, opts.delete, opts.secret);
    process.stderr.write("deleted\n");
    return;
  }

  const markdown = readMarkdown(opts.file);

  // Auto-detect difficulty from server
  let bits = opts.bits;
  try {
    const info = await fetchInfo(opts.url);
    bits = info.pow.bits;
  } catch {
    process.stderr.write(`warning: could not reach server, using default ${bits} bits\n`);
  }

  const stamp = computeStamp(bits);

  // Handle update / revise
  if (opts.update || opts.revise) {
    const id = opts.update || opts.revise;
    const result = await updatePitch(opts.url, id, opts.secret, stamp, markdown, opts.title, opts.author, opts.expires, opts.revise);
    console.log(result.url);
    return;
  }

  // Extract title: --title > frontmatter title > first h1 > "Untitled" (private)
  if (!opts.title) {
    const fmTitle = parseFrontmatterTitle(markdown);
    if (fmTitle) {
      opts.title = fmTitle;
    } else {
      const match = markdown.match(/^#\s+(.+)$/m);
      if (match) {
        opts.title = match[1].trim();
      } else {
        opts.title = "Untitled";
        opts.private = true;
      }
    }
  }

  const result = await submit(opts.url, stamp, markdown, opts.title, opts.slug, opts.author, opts.expires, opts.private);

  // Output URL to stdout, secret to stderr
  console.log(result.url);
  process.stderr.write(`secret: ${result.secret}\n`);

  if (result.expires_at) {
    process.stderr.write(`expires: ${result.expires_at}\n`);
  }
}

main().catch(e => die(e.message));
