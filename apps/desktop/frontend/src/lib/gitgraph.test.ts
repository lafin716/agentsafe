import { describe, expect, it } from "vitest";
import { layoutGraph, type GraphLayout, type GraphInput } from "./gitgraph";

// Builds commits from a compact "sha:parent parent" notation, newest first —
// the order git log --topo-order hands us.
function commits(...specs: string[]): GraphInput[] {
  return specs.map((spec) => {
    const [sha, parents = ""] = spec.split(":");
    return { sha, parents: parents.split(" ").filter(Boolean) };
  });
}

// Draws a layout the way the SVG gutter will, so the expectations below read as
// the picture a user would see. "*" is a node, "|" a line passing straight
// through a row, "/" and "\" a line changing lane within a row.
function draw(layout: GraphLayout<GraphInput>): string {
  return layout.rows
    .map((row) => {
      const cells = Array.from({ length: layout.laneCount }, () => " ");
      for (const link of row.links) {
        if (link.fromLane === link.toLane) {
          if (cells[link.fromLane] === " ") cells[link.fromLane] = "|";
        } else {
          const glyph = link.toLane < link.fromLane ? "/" : "\\";
          const at = link.toNode ? link.fromLane : link.toLane;
          if (cells[at] === " " || cells[at] === "|") cells[at] = glyph;
        }
      }
      cells[row.lane] = "*";
      return cells.join("");
    })
    .join("\n");
}

describe("layoutGraph", () => {
  it("puts a linear history in a single lane", () => {
    const layout = layoutGraph(commits("c:b", "b:a", "a"));

    expect(layout.laneCount).toBe(1);
    expect(layout.rows.map((r) => r.lane)).toEqual([0, 0, 0]);
    expect(draw(layout)).toBe(["*", "*", "*"].join("\n"));
  });

  it("returns an empty layout for no commits", () => {
    const layout = layoutGraph([]);

    expect(layout.rows).toEqual([]);
    expect(layout.laneCount).toBe(0);
  });

  it("gives a branch tip with no children in the window its own lane", () => {
    // Two independent tips over a shared base: the shape of a Base Branch and a
    // Feature branch that has diverged from it.
    const layout = layoutGraph(commits("feat:base", "main:base", "base"));

    expect(layout.rows.map((r) => r.lane)).toEqual([0, 1, 0]);
    expect(layout.laneCount).toBe(2);
    expect(draw(layout)).toBe(["* ", "|*", "*/"].join("\n"));
  });

  it("routes every child of a commit into that commit's node", () => {
    const layout = layoutGraph(commits("a:base", "b:base", "c:base", "base"));

    const base = layout.rows[3];
    expect(base.lane).toBe(0);
    // Three lanes converge; each arrives as a link that ends at the node.
    const converging = base.links.filter((l) => l.toNode);
    expect(converging).toHaveLength(3);
    expect(converging.map((l) => l.fromLane).sort()).toEqual([0, 1, 2]);
    expect(layout.laneCount).toBe(3);
  });

  it("allocates an extra lane for a merge commit's second parent", () => {
    const layout = layoutGraph(
      commits("merge:first second", "first:base", "second:base", "base")
    );

    expect(layout.rows[0].lane).toBe(0);
    const outgoing = layout.rows[0].links.filter((l) => l.fromNode);
    expect(outgoing).toHaveLength(2);
    // First parent keeps the merge's lane; the second gets a new one.
    expect(outgoing.map((l) => l.toLane).sort()).toEqual([0, 1]);
    expect(layout.rows[1].lane).toBe(0);
    expect(layout.rows[2].lane).toBe(1);
    expect(layout.laneCount).toBe(2);
  });

  it("reuses a lane once the branch occupying it has ended", () => {
    // side is a tip that merges away at base; later, another tip appears and
    // should take the freed lane rather than widening the graph.
    const layout = layoutGraph(
      commits("main:base", "side:base", "base:old", "old2:old", "old")
    );

    expect(layout.laneCount).toBe(2);
    // old2 has no children in the window, so it takes lane 1 — freed when side
    // converged into base — instead of lane 2.
    const old2 = layout.rows.find((r) => r.commit.sha === "old2");
    expect(old2?.lane).toBe(1);
  });

  it("keeps a lane open for a parent that falls outside the window", () => {
    // With a commit limit, an older tip's parent can be past the cut. The line
    // has to keep running to the bottom of the graph rather than vanishing.
    const layout = layoutGraph(commits("tip:cutoff"));

    expect(layout.rows).toHaveLength(1);
    const outgoing = layout.rows[0].links.filter((l) => l.fromNode);
    expect(outgoing).toHaveLength(1);
    expect(outgoing[0].toLane).toBe(0);
    expect(layout.danglingParents).toEqual(["cutoff"]);
  });

  it("marks a root commit as having no outgoing line", () => {
    const layout = layoutGraph(commits("only"));

    expect(layout.rows[0].links.filter((l) => l.fromNode)).toHaveLength(0);
    expect(layout.danglingParents).toEqual([]);
  });

  it("passes unrelated lanes straight through a row", () => {
    const layout = layoutGraph(
      commits("a:a2", "b:b2", "a2:base", "b2:base", "base")
    );

    // Row 2 is a2 in lane 0; b's lane 1 is waiting for b2 and must cross it.
    const row = layout.rows[2];
    expect(row.commit.sha).toBe("a2");
    const passing = row.links.filter(
      (l) => !l.fromNode && !l.toNode && l.fromLane === l.toLane
    );
    expect(passing.map((l) => l.fromLane)).toEqual([1]);
  });

  it("lays out a Feature branch over a moving Base Branch", () => {
    // The shape the graph page exists to show: origin/main has advanced twice
    // since feat/login branched off, and feat/login has two commits of its own.
    const layout = layoutGraph(
      commits(
        "login2:login1",
        "login1:fork",
        "main2:main1",
        "main1:fork",
        "fork"
      )
    );

    expect(layout.laneCount).toBe(2);
    expect(draw(layout)).toBe(
      ["* ", "* ", "|*", "|*", "*/"].join("\n")
    );
  });

  it("is stable when the same commit is listed twice", () => {
    // Defensive: a malformed read should not produce NaN lanes or loop.
    const layout = layoutGraph(commits("a:b", "a:b", "b"));

    expect(layout.rows).toHaveLength(3);
    for (const row of layout.rows) {
      expect(Number.isInteger(row.lane)).toBe(true);
      expect(row.lane).toBeGreaterThanOrEqual(0);
    }
  });
});
