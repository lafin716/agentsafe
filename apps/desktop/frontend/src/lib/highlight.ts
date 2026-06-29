// Syntax highlighting for the change/diff viewer. Uses highlight.js core with a
// small set of registered languages so the bundle stays small. The whole file is
// highlighted at once (so multi-line constructs like block comments, template
// literals and multi-line tags keep their context) and then the resulting HTML is
// re-tokenized into per-source-line token arrays. The diff viewer renders those
// tokens as React spans, layering its own change-highlight on top, so we never
// inject raw HTML (no dangerouslySetInnerHTML).

import hljs from "highlight.js/lib/core";
import xml from "highlight.js/lib/languages/xml";
import javascript from "highlight.js/lib/languages/javascript";
import typescript from "highlight.js/lib/languages/typescript";
import go from "highlight.js/lib/languages/go";
import python from "highlight.js/lib/languages/python";
import css from "highlight.js/lib/languages/css";
import scss from "highlight.js/lib/languages/scss";
import less from "highlight.js/lib/languages/less";
import json from "highlight.js/lib/languages/json";
import yaml from "highlight.js/lib/languages/yaml";
import markdown from "highlight.js/lib/languages/markdown";
import bash from "highlight.js/lib/languages/bash";
import sql from "highlight.js/lib/languages/sql";
import php from "highlight.js/lib/languages/php";
import java from "highlight.js/lib/languages/java";
import ruby from "highlight.js/lib/languages/ruby";
import rust from "highlight.js/lib/languages/rust";
import c from "highlight.js/lib/languages/c";
import cpp from "highlight.js/lib/languages/cpp";
import kotlin from "highlight.js/lib/languages/kotlin";
import swift from "highlight.js/lib/languages/swift";
import ini from "highlight.js/lib/languages/ini";
import dockerfile from "highlight.js/lib/languages/dockerfile";

import { splitLines, type WordSeg } from "./linediff";

// .vue is highlighted as xml/html — highlight.js' xml grammar handles the
// template and auto-highlights embedded <script> (JS) and <style> (CSS) blocks.
hljs.registerLanguage("xml", xml);
hljs.registerLanguage("javascript", javascript);
hljs.registerLanguage("typescript", typescript);
hljs.registerLanguage("go", go);
hljs.registerLanguage("python", python);
hljs.registerLanguage("css", css);
hljs.registerLanguage("scss", scss);
hljs.registerLanguage("less", less);
hljs.registerLanguage("json", json);
hljs.registerLanguage("yaml", yaml);
hljs.registerLanguage("markdown", markdown);
hljs.registerLanguage("bash", bash);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("php", php);
hljs.registerLanguage("java", java);
hljs.registerLanguage("ruby", ruby);
hljs.registerLanguage("rust", rust);
hljs.registerLanguage("c", c);
hljs.registerLanguage("cpp", cpp);
hljs.registerLanguage("kotlin", kotlin);
hljs.registerLanguage("swift", swift);
hljs.registerLanguage("ini", ini);
hljs.registerLanguage("dockerfile", dockerfile);

const EXT_LANG: Record<string, string> = {
  vue: "xml", html: "xml", htm: "xml", xml: "xml", svg: "xml",
  js: "javascript", jsx: "javascript", mjs: "javascript", cjs: "javascript",
  ts: "typescript", tsx: "typescript", mts: "typescript", cts: "typescript",
  go: "go",
  py: "python",
  css: "css", scss: "scss", sass: "scss", less: "less",
  json: "json",
  yaml: "yaml", yml: "yaml",
  md: "markdown", markdown: "markdown",
  sh: "bash", bash: "bash", zsh: "bash",
  sql: "sql",
  php: "php",
  java: "java",
  rb: "ruby",
  rs: "rust",
  c: "c", h: "c",
  cpp: "cpp", cc: "cpp", cxx: "cpp", hpp: "cpp", hh: "cpp", hxx: "cpp",
  kt: "kotlin", kts: "kotlin",
  swift: "swift",
  ini: "ini", toml: "ini", cfg: "ini", conf: "ini",
  dockerfile: "dockerfile",
};

// A run of text sharing the same highlight.js token class ("" = no token).
export interface Token {
  text: string;
  cls: string;
}

// Skip highlighting very large files — re-tokenizing huge HTML is not worth the
// jank (the backend already caps file views at 2 MB).
const MAX_HIGHLIGHT_BYTES = 300_000;

// languageForPath maps a file path to a registered highlight.js language, or
// returns undefined when we have no good grammar (caller falls back to plain).
export function languageForPath(path: string): string | undefined {
  if (!path) return undefined;
  const base = path.split(/[\\/]/).pop() ?? path;
  if (base.toLowerCase() === "dockerfile") return "dockerfile";
  const dot = base.lastIndexOf(".");
  if (dot < 0) return undefined;
  const ext = base.slice(dot + 1).toLowerCase();
  return EXT_LANG[ext];
}

function decodeEntity(ent: string): string {
  switch (ent) {
    case "&amp;": return "&";
    case "&lt;": return "<";
    case "&gt;": return ">";
    case "&quot;": return '"';
    case "&#x27;":
    case "&#39;": return "'";
    default: return ent;
  }
}

function plainLines(lines: string[]): Token[][] {
  return lines.map((line) => (line === "" ? [] : [{ text: line, cls: "" }]));
}

// highlightLines highlights the full text and returns one Token[] per source
// line (index i corresponds to the 1-based line number i+1, matching the diff
// cells produced by linediff). Falls back to plain (single-token) lines when no
// language is known, the file is too large, or highlighting throws.
export function highlightLines(text: string, language?: string): Token[][] {
  // Normalize line endings identically to linediff.splitLines so token-line
  // indexes line up with diff cell line numbers.
  const normalized = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const lines = splitLines(normalized);

  if (!language || !hljs.getLanguage(language) || normalized.length > MAX_HIGHLIGHT_BYTES) {
    return plainLines(lines);
  }

  let html: string;
  try {
    html = hljs.highlight(normalized, { language, ignoreIllegals: true }).value;
  } catch {
    return plainLines(lines);
  }

  const result: Token[][] = [];
  const stack: string[] = [];
  let current: Token[] = [];
  let buf = "";
  let i = 0;

  const topClass = () => (stack.length ? stack[stack.length - 1] : "");
  const flush = () => {
    if (buf) {
      current.push({ text: buf, cls: topClass() });
      buf = "";
    }
  };

  while (i < html.length) {
    const ch = html[i];
    if (ch === "<") {
      flush();
      if (html.startsWith("</span>", i)) {
        stack.pop();
        i += 7;
      } else if (html.startsWith("<span", i)) {
        const close = html.indexOf(">", i);
        const tag = html.slice(i, close + 1);
        const m = tag.match(/class="([^"]*)"/);
        stack.push(m ? m[1] : "");
        i = close + 1;
      } else {
        // highlight.js only emits <span> tags; treat anything else literally.
        buf += ch;
        i += 1;
      }
    } else if (ch === "\n") {
      flush();
      result.push(current);
      current = [];
      i += 1;
    } else if (ch === "&") {
      const semi = html.indexOf(";", i);
      if (semi !== -1 && semi - i <= 7) {
        buf += decodeEntity(html.slice(i, semi + 1));
        i = semi + 1;
      } else {
        buf += ch;
        i += 1;
      }
    } else {
      buf += ch;
      i += 1;
    }
  }
  flush();
  result.push(current);

  // Reconcile against splitLines: drop any trailing line a final newline added,
  // and guard against an unexpected mismatch by falling back per missing line.
  return lines.map((line, idx) => result[idx] ?? (line === "" ? [] : [{ text: line, cls: "" }]));
}

// A renderable piece carrying a single syntax token class and a single change
// flag, so the diff viewer can color it (cls) and tint it (changed) at once.
export interface Piece {
  text: string;
  cls: string;
  changed: boolean;
}

// composeLine overlays word-level change segments onto the syntax tokens of one
// line, splitting at both boundaries so every piece has exactly one token class
// and one changed flag. Token texts and segment texts both reconstruct the same
// line, so their lengths line up.
export function composeLine(tokens: Token[], segs?: WordSeg[]): Piece[] {
  if (!segs || segs.length === 0) {
    return tokens.map((t) => ({ text: t.text, cls: t.cls, changed: false }));
  }
  const pieces: Piece[] = [];
  let si = 0; // current segment index
  let segOffset = 0; // chars already consumed from the current segment
  for (const tok of tokens) {
    let remaining = tok.text;
    while (remaining.length > 0 && si < segs.length) {
      const seg = segs[si];
      const take = Math.min(remaining.length, seg.text.length - segOffset);
      pieces.push({ text: remaining.slice(0, take), cls: tok.cls, changed: seg.changed });
      remaining = remaining.slice(take);
      segOffset += take;
      if (segOffset >= seg.text.length) {
        si++;
        segOffset = 0;
      }
    }
    if (remaining.length > 0) {
      pieces.push({ text: remaining, cls: tok.cls, changed: false });
    }
  }
  return pieces;
}
