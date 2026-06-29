// Minimal side-by-side line diff for the change viewer. Produces rows that align
// the old (left) and new (right) lines so both columns scroll together, each row
// tagged equal/added/removed/changed for highlighting. Uses an LCS over lines
// with a size guard so very large files fall back to a simple unaligned diff.

export type DiffRowType = "equal" | "added" | "removed" | "changed";

export interface DiffCell {
  n: number; // 1-based line number on that side
  text: string;
}

export interface DiffRow {
  type: DiffRowType;
  left?: DiffCell;
  right?: DiffCell;
}

// Above this old×new product the O(n*m) LCS table is too costly; fall back.
const MAX_LCS_PRODUCT = 4_000_000;

export function splitLines(text: string): string[] {
  if (text === "") return [];
  const normalized = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const lines = normalized.split("\n");
  // Drop the single trailing empty line a final newline produces.
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

type Op = { type: "equal" | "del" | "ins"; oldIdx?: number; newIdx?: number };

function lcsOps(a: string[], b: string[]): Op[] {
  const n = a.length;
  const m = b.length;
  const w = m + 1;
  // dp[i*w+j] = LCS length of a[i:] and b[j:].
  const dp = new Int32Array((n + 1) * w);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * w + j] =
        a[i] === b[j]
          ? dp[(i + 1) * w + (j + 1)] + 1
          : Math.max(dp[(i + 1) * w + j], dp[i * w + (j + 1)]);
    }
  }
  const ops: Op[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      ops.push({ type: "equal", oldIdx: i, newIdx: j });
      i++;
      j++;
    } else if (dp[(i + 1) * w + j] >= dp[i * w + (j + 1)]) {
      ops.push({ type: "del", oldIdx: i });
      i++;
    } else {
      ops.push({ type: "ins", newIdx: j });
      j++;
    }
  }
  while (i < n) {
    ops.push({ type: "del", oldIdx: i });
    i++;
  }
  while (j < m) {
    ops.push({ type: "ins", newIdx: j });
    j++;
  }
  return ops;
}

function fallbackRows(a: string[], b: string[]): DiffRow[] {
  const rows: DiffRow[] = [];
  const max = Math.max(a.length, b.length);
  for (let k = 0; k < max; k++) {
    const left = k < a.length ? { n: k + 1, text: a[k] } : undefined;
    const right = k < b.length ? { n: k + 1, text: b[k] } : undefined;
    let type: DiffRowType = "equal";
    if (left && right) type = left.text === right.text ? "equal" : "changed";
    else if (left) type = "removed";
    else type = "added";
    rows.push({ type, left, right });
  }
  return rows;
}

// diffRows computes the aligned side-by-side diff of two texts (old → new).
export function diffRows(oldText: string, newText: string): DiffRow[] {
  const a = splitLines(oldText);
  const b = splitLines(newText);
  if (a.length + b.length === 0) return [];
  if (a.length * b.length > MAX_LCS_PRODUCT) return fallbackRows(a, b);

  const ops = lcsOps(a, b);
  const rows: DiffRow[] = [];
  let k = 0;
  while (k < ops.length) {
    const op = ops[k];
    if (op.type === "equal") {
      rows.push({
        type: "equal",
        left: { n: op.oldIdx! + 1, text: a[op.oldIdx!] },
        right: { n: op.newIdx! + 1, text: b[op.newIdx!] },
      });
      k++;
      continue;
    }
    // Pair a run of deletions with the following insertions as "changed" rows;
    // leftovers become one-sided removed/added rows.
    const dels: Op[] = [];
    const inss: Op[] = [];
    while (k < ops.length && ops[k].type === "del") dels.push(ops[k++]);
    while (k < ops.length && ops[k].type === "ins") inss.push(ops[k++]);
    const pairs = Math.min(dels.length, inss.length);
    for (let p = 0; p < pairs; p++) {
      rows.push({
        type: "changed",
        left: { n: dels[p].oldIdx! + 1, text: a[dels[p].oldIdx!] },
        right: { n: inss[p].newIdx! + 1, text: b[inss[p].newIdx!] },
      });
    }
    for (let p = pairs; p < dels.length; p++) {
      rows.push({ type: "removed", left: { n: dels[p].oldIdx! + 1, text: a[dels[p].oldIdx!] } });
    }
    for (let p = pairs; p < inss.length; p++) {
      rows.push({ type: "added", right: { n: inss[p].newIdx! + 1, text: b[inss[p].newIdx!] } });
    }
  }
  return rows;
}

// A run of characters within a single line, flagged as changed (differs from the
// other side) or unchanged. Used to emphasize only the parts of a changed line
// that actually differ, GitHub side-by-side style.
export interface WordSeg {
  text: string;
  changed: boolean;
}

export interface WordDiff {
  left: WordSeg[];
  right: WordSeg[];
}

// Tokenize a line into words / whitespace / single punctuation so word-level diffs
// align on natural boundaries instead of arbitrary characters.
function tokenizeWords(s: string): string[] {
  return s.match(/(\s+|[A-Za-z0-9_]+|[^\sA-Za-z0-9_])/g) ?? [];
}

// Above this token product the O(n*m) LCS is skipped (whole middle is "changed").
const MAX_WORD_LCS_PRODUCT = 250_000;

function lcsWordChanges(a: string[], b: string[]): { aChanged: boolean[]; bChanged: boolean[] } {
  const n = a.length;
  const m = b.length;
  const aChanged = new Array<boolean>(n).fill(true);
  const bChanged = new Array<boolean>(m).fill(true);
  if (n === 0 || m === 0 || n * m > MAX_WORD_LCS_PRODUCT) {
    return { aChanged, bChanged };
  }
  const w = m + 1;
  const dp = new Int32Array((n + 1) * w);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * w + j] =
        a[i] === b[j]
          ? dp[(i + 1) * w + (j + 1)] + 1
          : Math.max(dp[(i + 1) * w + j], dp[i * w + (j + 1)]);
    }
  }
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      aChanged[i] = false;
      bChanged[j] = false;
      i++;
      j++;
    } else if (dp[(i + 1) * w + j] >= dp[i * w + (j + 1)]) {
      i++;
    } else {
      j++;
    }
  }
  return { aChanged, bChanged };
}

function mergeSegs(tokens: string[], flags: boolean[]): WordSeg[] {
  const segs: WordSeg[] = [];
  for (let k = 0; k < tokens.length; k++) {
    const changed = flags[k];
    const last = segs[segs.length - 1];
    if (last && last.changed === changed) last.text += tokens[k];
    else segs.push({ text: tokens[k], changed });
  }
  return segs;
}

// wordDiff returns per-side character segments flagging the parts of a changed
// line that actually differ. Common prefix/suffix tokens are trimmed first, then a
// small LCS over the middle marks only genuinely changed runs.
export function wordDiff(oldLine: string, newLine: string): WordDiff {
  if (oldLine === newLine) {
    return {
      left: oldLine ? [{ text: oldLine, changed: false }] : [],
      right: newLine ? [{ text: newLine, changed: false }] : [],
    };
  }
  const a = tokenizeWords(oldLine);
  const b = tokenizeWords(newLine);

  let p = 0;
  while (p < a.length && p < b.length && a[p] === b[p]) p++;
  let sa = a.length;
  let sb = b.length;
  while (sa > p && sb > p && a[sa - 1] === b[sb - 1]) {
    sa--;
    sb--;
  }

  const leftFlags = new Array<boolean>(a.length).fill(false);
  const rightFlags = new Array<boolean>(b.length).fill(false);
  const { aChanged, bChanged } = lcsWordChanges(a.slice(p, sa), b.slice(p, sb));
  for (let k = 0; k < aChanged.length; k++) if (aChanged[k]) leftFlags[p + k] = true;
  for (let k = 0; k < bChanged.length; k++) if (bChanged[k]) rightFlags[p + k] = true;

  return { left: mergeSegs(a, leftFlags), right: mergeSegs(b, rightFlags) };
}
