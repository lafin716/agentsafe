// Lane assignment for a Commit Graph.
//
// The Go side returns commits with their parent SHAs and nothing about layout:
// where a line sits is a presentation concern, and keeping it here means the
// algorithm is a pure function with no Git or Wails dependency to test around.
//
// Input order matters. Commits must arrive newest-first in topological order,
// which is what `git log --topo-order` produces.

export type GraphInput = {
  sha: string;
  parents: string[];
};

/**
 * One line segment crossing a row. `fromLane` is where the line sits at the
 * row's top edge and `toLane` where it sits at the bottom edge; the node itself
 * is at the row's vertical middle, so a link that touches it is marked with
 * `fromNode` (it leaves the node, heading down to a parent) or `toNode` (it
 * arrives at the node from a child above).
 */
export type GraphLink = {
  fromLane: number;
  toLane: number;
  fromNode: boolean;
  toNode: boolean;
};

export type GraphRow<T extends GraphInput> = {
  commit: T;
  lane: number;
  links: GraphLink[];
};

export type GraphLayout<T extends GraphInput> = {
  rows: GraphRow<T>[];
  /** Widest point of the graph, so the gutter can be sized once. */
  laneCount: number;
  /**
   * Parents referenced by a commit in the window but not present in it —
   * history past the commit limit. Their lanes stay open so the lines run off
   * the bottom of the graph instead of stopping mid-air.
   */
  danglingParents: string[];
};

export function layoutGraph<T extends GraphInput>(
  commits: T[]
): GraphLayout<T> {
  // lanes[i] holds the SHA that lane i is currently waiting to reach, or null
  // when the lane is free for reuse.
  const lanes: (string | null)[] = [];
  const present = new Set(commits.map((c) => c.sha));
  const dangling = new Set<string>();
  const rows: GraphRow<T>[] = [];
  let laneCount = 0;

  const claimFreeLane = (sha: string): number => {
    const free = lanes.indexOf(null);
    if (free >= 0) {
      lanes[free] = sha;
      return free;
    }
    lanes.push(sha);
    return lanes.length - 1;
  };

  for (const commit of commits) {
    const links: GraphLink[] = [];

    // Every lane waiting for this commit converges on its node. The leftmost
    // one becomes the commit's own lane so the graph leans left; the rest are
    // freed for reuse below.
    const arriving: number[] = [];
    for (let i = 0; i < lanes.length; i += 1) {
      if (lanes[i] === commit.sha) arriving.push(i);
    }
    const lane = arriving.length > 0 ? arriving[0] : claimFreeLane(commit.sha);

    for (let i = 0; i < lanes.length; i += 1) {
      if (lanes[i] === null) continue;
      if (lanes[i] === commit.sha) {
        links.push({ fromLane: i, toLane: lane, fromNode: false, toNode: true });
        // Freed now; the outgoing links below re-claim what they need.
        if (i !== lane) lanes[i] = null;
        continue;
      }
      // An unrelated branch crossing this row.
      links.push({ fromLane: i, toLane: i, fromNode: false, toNode: false });
    }

    // The commit's lane carries its first parent onward; additional parents of a
    // merge each need a lane of their own.
    commit.parents.forEach((parent, index) => {
      if (!present.has(parent)) dangling.add(parent);
      const target = index === 0 ? lane : claimFreeLane(parent);
      lanes[target] = parent;
      links.push({
        fromLane: lane,
        toLane: target,
        fromNode: true,
        toNode: false,
      });
    });
    if (commit.parents.length === 0) lanes[lane] = null;

    rows.push({ commit, lane, links });
    laneCount = Math.max(laneCount, lanes.length, lane + 1);
  }

  return { rows, laneCount, danglingParents: [...dangling] };
}

/**
 * Horizontal centre of a lane in the SVG gutter, in pixels.
 */
export function laneX(lane: number, laneWidth: number): number {
  return laneWidth * lane + laneWidth / 2;
}

/**
 * SVG path for one link across a row of height `rowHeight`. A link that stays in
 * its lane is a straight segment; one that changes lane is a cubic curve, which
 * reads better than a diagonal at the row heights we use.
 */
export function linkPath(
  link: GraphLink,
  rowHeight: number,
  laneWidth: number
): string {
  const x1 = laneX(link.fromLane, laneWidth);
  const x2 = laneX(link.toLane, laneWidth);
  const mid = rowHeight / 2;
  const y1 = link.fromNode ? mid : 0;
  const y2 = link.toNode ? mid : rowHeight;
  if (x1 === x2) return `M ${x1} ${y1} L ${x2} ${y2}`;
  const cy = (y1 + y2) / 2;
  return `M ${x1} ${y1} C ${x1} ${cy}, ${x2} ${cy}, ${x2} ${y2}`;
}

// Lane colours cycle through a fixed palette so a branch keeps its colour while
// the user scrolls. The values are theme custom properties, so the graph follows
// light/dark mode without a second palette living here.
const LANE_COUNT = 6;

export function laneColor(lane: number): string {
  const index = ((lane % LANE_COUNT) + LANE_COUNT) % LANE_COUNT;
  return `hsl(var(--graph-lane-${index + 1}))`;
}
