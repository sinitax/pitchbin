#!/usr/bin/env node

// Integration tests for the pitchbin CLI.
// Requires a running pitchbin server at PITCHBIN_URL (default http://localhost:18956).
// Run: node test.mjs

import { createHash, randomBytes } from "node:crypto";
import { execSync } from "node:child_process";
import { writeFileSync, unlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const BASE = process.env.PITCHBIN_URL || "http://localhost:18956";
let passed = 0;
let failed = 0;

function solveStamp(bits) {
  const ts = Math.floor(Date.now() / 1000);
  const rand = randomBytes(8).toString("hex");
  const prefix = `pitchbin:1:${ts}:${rand}:`;
  for (let nonce = 0; ; nonce++) {
    const stamp = prefix + nonce;
    const hash = createHash("sha256").update(stamp).digest();
    const fullBytes = bits >> 3;
    const remainBits = bits & 7;
    let ok = true;
    for (let i = 0; i < fullBytes; i++) { if (hash[i] !== 0) { ok = false; break; } }
    if (ok && remainBits > 0 && (hash[fullBytes] & (0xFF << (8 - remainBits))) !== 0) ok = false;
    if (ok) return stamp;
  }
}

function assert(cond, msg) {
  if (!cond) {
    console.error(`  FAIL: ${msg}`);
    failed++;
  } else {
    passed++;
  }
}

function cli(args, opts = {}) {
  const env = { ...process.env, PITCHBIN_URL: BASE };
  const cmd = `node ${join(import.meta.dirname, "pitchbin.mjs")} ${args}`;
  try {
    const out = execSync(cmd, { env, input: opts.stdin, encoding: "utf-8", timeout: 30000 });
    return { stdout: out.trim(), code: 0 };
  } catch (e) {
    return { stdout: (e.stdout || "").trim(), stderr: (e.stderr || "").trim(), code: e.status };
  }
}

async function fetchJSON(path) {
  const r = await fetch(`${BASE}${path}`);
  return { status: r.status, body: await r.json() };
}

async function fetchText(path) {
  const r = await fetch(`${BASE}${path}`);
  return { status: r.status, body: await r.text() };
}

// --- Tests ---

console.log("Testing pitchbin CLI against", BASE);
console.log();

// Test: help flag
console.log("test: --help");
{
  const r = cli("--help");
  assert(r.code === 0, "exit code 0");
  assert(r.stdout.includes("Usage:"), "shows usage");
}

// Test: missing file argument
console.log("test: missing file");
{
  const r = cli("");
  assert(r.code !== 0, "should fail");
}

// Test: submit from stdin
console.log("test: submit from stdin");
{
  const r = cli('--title "CLI Test" --author "testbot" --bits 8 -', { stdin: "# Hello from stdin\n\nTest body." });
  assert(r.code === 0, `exit code 0 (got ${r.code})`);
  assert(r.stdout.startsWith(BASE), `URL starts with base (got ${r.stdout})`);
  assert(r.stdout.includes("/cli-test"), `slug in URL (got ${r.stdout})`);

  // Verify via API
  const id = r.stdout.replace(BASE + "/", "");
  const raw = await fetchText(`/${id}/raw`);
  assert(raw.status === 200, "raw endpoint 200");
  assert(raw.body.includes("Hello from stdin"), "raw content matches");
}

// Test: submit from file
console.log("test: submit from file");
{
  const tmp = join(tmpdir(), `pitchbin-test-${Date.now()}.md`);
  writeFileSync(tmp, "# File Test\n\nFrom a temp file.");
  const r = cli(`--title "File Test" --bits 8 ${tmp}`);
  assert(r.code === 0, "exit code 0");
  assert(r.stdout.startsWith(BASE), "URL starts with base");
  unlinkSync(tmp);
}

// Test: private flag
console.log("test: --private flag");
{
  const r = cli('--title "Private Test" --private --bits 8 -', { stdin: "# Private" });
  assert(r.code === 0, "exit code 0");
  const id = r.stdout.replace(BASE + "/", "");
  // Private IDs have a random suffix after the slug
  assert(id.length > "private-test-".length, `private id has random suffix (got ${id})`);
}

// Test: slug collision
console.log("test: slug collision");
{
  const r1 = cli('--title "Dupe" --bits 8 -', { stdin: "# first" });
  const r2 = cli('--title "Dupe" --bits 8 -', { stdin: "# second" });
  assert(r1.code === 0 && r2.code === 0, "both succeed");
  assert(r1.stdout !== r2.stdout, `different URLs (${r1.stdout} vs ${r2.stdout})`);
  assert(r2.stdout.includes("dupe-2"), `second gets -2 suffix (got ${r2.stdout})`);
}

// Test: unknown flag
console.log("test: unknown flag");
{
  const r = cli("--bogus");
  assert(r.code !== 0, "should fail");
}

// Test: annotations roundtrip
console.log("test: annotations API");
{
  const r = cli('--title "Annotate Me" --bits 8 -', { stdin: "# Annotate\n\nSome text to annotate." });
  assert(r.code === 0, "pitch created");
  const id = r.stdout.replace(BASE + "/", "");

  // Compute a PoW stamp for the annotation
  const stamp = solveStamp(8);

  // Post annotation
  const post = await fetch(`${BASE}/api/${id}/annotations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ stamp, author: "tester", comment: "looks good", quote: "Some text", text_start: 12, text_end: 21 }),
  });
  assert(post.status === 201, `post annotation 201 (got ${post.status})`);

  // Get annotations
  const get = await fetchJSON(`/api/${id}/annotations`);
  assert(get.status === 200, "get annotations 200");
  assert(get.body.length === 1, `one annotation (got ${get.body.length})`);
  assert(get.body[0].author === "tester", "author matches");
}

// --- Summary ---
console.log();
console.log(`${passed} passed, ${failed} failed`);
process.exit(failed > 0 ? 1 : 0);
