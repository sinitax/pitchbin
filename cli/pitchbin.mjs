#!/usr/bin/env node

import { createHash, randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";

const USAGE = `Usage: pitchbin [options] <file|->`

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
    private: false,
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
  --expires SPEC   Expiry: 7d, 30d, 90d (default: permanent)
  --private        Add random suffix to URL (unguessable)
  --bits N         PoW difficulty override (default: auto-detect from server)
  -                Read markdown from stdin`);
        process.exit(0);
      case "--url": opts.url = args[++i]; break;
      case "--title": opts.title = args[++i]; break;
      case "--author": opts.author = args[++i]; break;
      case "--expires": opts.expires = args[++i]; break;
      case "--bits": opts.bits = parseInt(args[++i], 10); break;
      case "-p": case "--private": opts.private = true; break;
      default:
        if (args[i].startsWith("-") && args[i] !== "-") die(`unknown flag: ${args[i]}`);
        opts.file = args[i];
    }
  }

  if (!opts.file) die("missing file argument. Use - for stdin.");
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

async function submit(url, stamp, markdown, title, author, expires, isPrivate) {
  const resp = await fetch(`${url}/api/pitch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ stamp, markdown, title, author, expires, private: isPrivate }),
  });

  const body = await resp.json();
  if (!resp.ok) die(`submission failed: ${body.error || resp.status}`);
  return body;
}

async function main() {
  const opts = parseArgs();
  const markdown = readMarkdown(opts.file);

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

  // Auto-detect difficulty from server
  let bits = opts.bits;
  try {
    const info = await fetchInfo(opts.url);
    bits = info.pow.bits;
  } catch {
    process.stderr.write(`warning: could not reach server, using default ${bits} bits\n`);
  }

  const stamp = computeStamp(bits);
  const result = await submit(opts.url, stamp, markdown, opts.title, opts.author, opts.expires, opts.private);

  // Output just the URL to stdout (for piping)
  console.log(result.url);

  if (result.expires_at) {
    process.stderr.write(`expires: ${result.expires_at}\n`);
  }
}

main().catch(e => die(e.message));
