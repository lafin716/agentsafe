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

function splitLines(text: string): string[] {
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
