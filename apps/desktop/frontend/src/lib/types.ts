// TypeScript mirrors of the Go structs returned by the Wails bindings.
// NOTE: config.* structs have no json tags, so encoding/json serializes them
// with PascalCase Go field names. The feature/agent structs DO have json tags
// (camelCase), and the desktop wrapper types use camelCase json tags too.

export interface Workspace {
  Name: string;
  Root: string;
}

// A workspace registered with the app (registry.Entry, camelCase json tags).
export interface WorkspaceEntry {
  name: string;
  path: string;
}

export interface GitConfig {
  DefaultBaseBranch: string;
  BranchPrefix: string;
  // Pattern used for the commit message when a delivery action runs without an
  // explicit one. Empty means the built-in default. Only sync → commit → push
  // uses it; the commit buttons still require a message the user typed.
  CommitMessageTemplate?: string;
}

export interface Repository {
  Name: string;
  URL: string;
  DefaultBranch: string;
  TestCommand: string;
}

export interface AgentConfig {
  SecurityFileName: string;
  // Legacy split-file names, retained for backward compatibility.
  IgnoreFileName?: string;
  MaskFileName?: string;
  DefaultExclude: string[] | null;
  // When true/unset, prepare/diff/sync also honor the feature worktree's
  // .gitignore so agent build output is not synced back.
  RespectGitignore?: boolean | null;
}

export interface GitLabConfig {
  BaseURL: string;
  TokenEnv: string;
  TargetBranch: string;
}

export interface GitHubConfig {
  TokenEnv: string;
  TargetBranch: string;
}

export interface Config {
  Version: number;
  Workspace: Workspace;
  Git: GitConfig;
  Repositories: Repository[] | null;
  Agent: AgentConfig;
  GitLab: GitLabConfig;
  GitHub: GitHubConfig;
}

export type WorktreeTemplateTargetMode =
  | "workspaceRoot"
  | "featureRoot"
  | "allRepos"
  | "selectedRepos"
  | "agentRoot"
  | "agentAllRepos"
  | "agentSelectedRepos"
  | string;

export interface WorktreeTemplate {
  id: string;
  name: string;
  sourcePath: string;
  enabled: boolean;
  targetMode: WorktreeTemplateTargetMode;
  repoNames: string[] | null;
  overwrite: boolean;
}

export interface WorktreeTemplateTreeNode {
  name: string;
  relPath: string;
  isDir: boolean;
  size: number;
  files: number;
  folders: number;
  children: WorktreeTemplateTreeNode[] | null;
}

export interface WorktreeTemplateTree {
  template: WorktreeTemplate;
  root: WorktreeTemplateTreeNode;
}

// Resolved for the one path selected in the File Explorer, not for the whole
// tree: templateId is set when the path is already a worktree template source.
export interface WorkspacePathState {
  tracked: boolean;
  templateId?: string;
}

export interface WorkspaceTreeNode {
  name: string;
  path: string;
  relPath: string;
  isDir: boolean;
  size: number;
  modTime: string;
  featureName?: string;
  branch?: string;
  // Set when this node came from a worktree template; templateModified is true
  // when a template file's current content differs from the stored template.
  templateId?: string;
  templateRelPath?: string;
  templateModified?: boolean;
  children: WorkspaceTreeNode[] | null;
}

export interface RepoRuntimeState {
  name: string;
  local: boolean;
  currentBranch: string;
  remoteBranches: string[] | null;
  error?: string;
}

export interface FeatureEntry {
  name: string;
  branch: string;
  baseBranch: string;
  repoCount: number;
  agentReady: boolean;
}

export interface FeatureListResult {
  features: FeatureEntry[] | null;
}

export interface RepositoryCreateCheck {
  name: string;
  baseBranch: string;
  localBranch: boolean;
  remoteBranch: boolean;
  checkedOutAt?: string;
  conflict: boolean;
  canReuse: boolean;
  canRecreate: boolean;
  blockedReason?: string;
}

export interface FeatureCreateCheck {
  name: string;
  branch: string;
  hasConflicts: boolean;
  blocked: boolean;
  repositories: RepositoryCreateCheck[] | null;
}

export interface RepoStatus {
  name: string;
  status: string;
  changes: RepoFileStatus[] | null;
  // Number of commits not yet pushed (what a push would publish).
  ahead?: number;
  error?: string;
  agentReady?: boolean;
  agentNeedsPrepare?: boolean;
  // Any Interrupted Integration open in this repo worktree. Mid-rebase the HEAD
  // is detached, so `branch` reads as empty — show this instead of a blank.
  integration?: IntegrationState;
  // The feature branch this repo worktree has checked out, and the base branch
  // it sits on top of. The status rows used to show the repository's configured
  // default branch here, which is the base — not what is checked out.
  branch?: string;
  baseBranch?: string;
  // HEAD of this repo worktree. Absent on a branch with no commits yet.
  lastCommit?: GraphCommit;
}

export interface RepoFileStatus {
  code: string;
  type: "added" | "modified" | "deleted" | "renamed" | "conflict" | "other" | string;
  path: string;
}

export interface FeatureStatusResult {
  feature: string;
  branch: string;
  agentReady?: boolean;
  agentNeedsPrepare?: boolean;
  repositories: RepoStatus[] | null;
}

// A rebase or merge that stopped on conflict, leaving a repo worktree in a
// partial state. Left in place rather than aborted so it can be resolved —
// see docs/adr/0002-integration-conflicts-are-left-in-place.md.
export interface IntegrationState {
  kind?: "rebase" | "merge" | "";
  // Branch being replayed (rebase) or merged into (merge).
  branch?: string;
  // Commit being replayed onto, or merged in. Always a raw SHA.
  onto?: string;
  // Rebase progress; both absent for a merge.
  step?: number;
  total?: number;
  summary?: string;
  conflictPaths: string[] | null;
}

export function integrationInProgress(
  state: IntegrationState | undefined
): boolean {
  return !!state?.kind;
}

export type IntegrationStatus =
  | "rebased"
  | "merged"
  | "up-to-date"
  | "skipped"
  | "conflicted"
  | "failed"
  | "continued"
  | "aborted"
  | string;

export interface IntegrationRepoResult {
  name: string;
  branch: string;
  baseBranch: string;
  // The ref actually integrated from: origin/<base> when it resolves, else local.
  upstream?: string;
  status: IntegrationStatus;
  detail: string;
  // Unmerged paths when status is "conflicted".
  conflicts?: string[] | null;
  // The failing command with its stdout and stderr, verbatim. Kept apart from
  // `detail` so a row can show the summary and put this behind a toggle.
  gitOutput?: string;
}

export interface IntegrationResult {
  feature: string;
  // "rebase" | "merge" | "continue" | "abort"
  operation: string;
  repositories: IntegrationRepoResult[] | null;
}

// Whether a rebase or merge may run, per repository — shown in the confirm
// dialog before anything changes.
export interface RepoIntegrationReadiness {
  repo: string;
  agentPrepared: boolean;
  // Unreviewed Agent Changes found. Integrating would let a later sync overwrite
  // the result, so any is blocking.
  agentChanges: number;
  // Integrating will leave the agent workspace needing to be prepared again.
  staleAfter: boolean;
  blocked: boolean;
  reason?: string;
}

export interface IntegrationReadiness {
  feature: string;
  repositories: RepoIntegrationReadiness[] | null;
}

export type RefKind = "head" | "remote" | "tag";

export interface GraphRef {
  name: string;
  kind: RefKind;
}

export interface RefTip {
  name: string;
  kind: RefKind;
  sha: string;
}

export interface GraphCommit {
  sha: string;
  parents: string[];
  authorName: string;
  authorEmail: string;
  authorDate: string;
  subject: string;
  refs: GraphRef[] | null;
  isHead: boolean;
}

// A branch in the graph that has a repo worktree, which is what makes it live
// work rather than just a name.
export interface BranchWorktree {
  branch: string;
  feature: string;
  // Relative to the workspace root.
  path: string;
  integration: IntegrationState;
  baseBranch: string;
}

export interface CommitGraph {
  repo: string;
  baseBranch: string;
  // What the main clone has checked out. Never integrated into (docs/adr/0001);
  // shown so the user can see which branch a Pull would fast-forward.
  currentBranch: string;
  commits: GraphCommit[] | null;
  refs: RefTip[] | null;
  worktrees: BranchWorktree[] | null;
  // Refs whose tip is older than the commit limit, so they are not drawn.
  // Reporting them is what stops the graph reading as "this branch is gone".
  outsideWindow: RefTip[] | null;
  limit: number;
  allBranches: boolean;
  truncated: boolean;
}

// Push outcome per repository. A push that failed in one repository is not a
// failure of the whole operation, which is why this is a result rather than a
// thrown error — reporting "pushed" while every repository failed was the bug.
export type PushStatus = "pushed" | "up-to-date" | "skipped" | "failed" | string;

export interface PushRepoResult {
  name: string;
  branch: string;
  status: PushStatus;
  detail: string;
  // Whether this repository went up with --force-with-lease.
  forced: boolean;
  // Human-readable failure summary, and the raw git command with its output.
  error?: string;
  gitOutput?: string;
}

export interface PushResult {
  feature: string;
  repositories: PushRepoResult[] | null;
}

// A rebase together with the pushes that followed it. Which repositories get
// pushed is decided in internal/feature — only the ones whose history was
// actually rewritten (docs/adr/0003).
export interface IntegrationPushResult {
  integration: IntegrationResult;
  pushes?: PushResult[] | null;
}

// What a push would send, per repository, with the range each count came from.
// The badge and this list resolve the range the same way, so they cannot
// disagree about what "unpushed" means.
export interface UnpushedRepo {
  name: string;
  branch: string;
  // e.g. "origin/feature/x..HEAD". Empty when no comparison point was found.
  range: string;
  count: number;
  commits: GraphCommit[] | null;
  error?: string;
}

export interface UnpushedResult {
  feature: string;
  repositories: UnpushedRepo[] | null;
}

// Read-only inspection shown while the rebase dialog fills itself in. Opening
// the dialog changes nothing.
export interface RepoRebasePreflight {
  repo: string;
  branch: string;
  baseBranch: string;
  // The ref the rebase would replay onto, preferring origin/<base>. Absent when
  // none resolved, in which case `reason` says so.
  upstream?: string;
  // Commits the upstream has that this branch does not — how much a rebase
  // would replay over. Zero means already up to date.
  behind: number;
  // Commits that would need force-pushing afterwards.
  unpushed: number;
  // Dirty repo worktrees are skipped rather than stashed.
  dirty: boolean;
  integration: IntegrationState;
  // Unreviewed Agent Changes; any is blocking, because a later sync would
  // overwrite the rebase.
  agentChanges: number;
  blocked: boolean;
  reason?: string;
}

export interface RebasePreflight {
  feature: string;
  repositories: RepoRebasePreflight[] | null;
}

// The workspace's Commit Message Template, with what it currently produces.
export interface CommitMessageTemplateInfo {
  template: string;
  variables: string[];
  // Shown as the commit field's placeholder rather than prefilled, so
  // {{timestamp}} is not frozen to whenever the screen opened.
  preview: string;
  // Set when the saved template cannot be rendered; preview then shows the
  // built-in default that would be used instead.
  error?: string;
}

export interface CommitFileChange {
  // git's name-status letter: A, M, D, R, C or T.
  status: string;
  path: string;
  // Set for renames and copies.
  oldPath?: string;
}

export interface FeatureDeleteResult {
  warnings: string[] | null;
}

export type MaskRuleType = "plain" | "regex" | "keypath" | string;

export interface MaskRule {
  name: string;
  type: MaskRuleType;
  pattern: string;
  replacement: string;
}

export interface MaskFile {
  rules: MaskRule[] | null;
}

export type PreviewStatus = "ignored" | "masked" | "copied";

export interface MaskMatch {
  name: string;
  type: MaskRuleType;
  pattern: string;
  count: number;
}

export interface PreviewEntry {
  path: string;
  isDir: boolean;
  status: PreviewStatus;
  ignorePattern?: string;
  maskMatches?: MaskMatch[] | null;
  replacements?: number;
  binary?: boolean;
}

export interface PreviewResult {
  repo: string;
  source: string;
  ignored: number;
  masked: number;
  copied: number;
  total: number;
  entries: PreviewEntry[] | null;
}

export interface SecurityPreviewFile {
  before: FileViewSide;
  after: FileViewSide;
}

export interface SecurityTemplate {
  key: string;
  label: string;
  description: string;
  ignoreCount: number;
  maskCount: number;
}

export interface RepoMeta {
  name: string;
  worktreePath: string;
  branch: string;
  baseBranch: string;
  revision?: number;
}

export interface FeatureMetadata {
  name: string;
  branch: string;
  baseBranch: string;
  revision?: number;
  createdAt: string;
  repositories: RepoMeta[] | null;
}

export interface FeaturePathRepo {
  name: string;
  worktreePath: string;
  agentPath: string;
}

export interface FeaturePathsResult {
  feature: string;
  worktreePath: string;
  agentPath: string;
  repositories: FeaturePathRepo[] | null;
}

export interface PrepareRepo {
  name: string;
  source: string;
  agent: string;
  copiedFiles: number;
  ignoredFiles: number;
  maskedFiles: string[] | null;
  preparedHashes?: Record<string, string>;
  worktreeRevision?: number;
}

export interface PrepareMetadata {
  feature: string;
  preparedAt: string;
  featureRevision?: number;
  repositories: PrepareRepo[] | null;
}

export interface BackupEntry {
  feature: string;
  repo: string;
  path: string;
  createdAt: string;
  size: number;
  files: number;
}

export interface SyncChange {
  path: string;
  type: string; // ADDED | MODIFIED | DELETED
}

export interface SyncHistoryEntry {
  id: string;
  feature: string;
  repo: string;
  syncedAt: string;
  changeCount: number;
  changes: SyncChange[] | null;
  canRollback: boolean;
}

export type ChangeType = "added" | "modified" | "deleted" | string;

export interface Change {
  repo: string;
  type: ChangeType;
  path: string;
  risky: boolean;
  masked: boolean;
}

export interface RepoDiff {
  name: string;
  changes: Change[] | null;
}

export interface DiffResult {
  feature: string;
  repositories: RepoDiff[] | null;
}

export interface FileViewSide {
  path: string;
  exists: boolean;
  content?: string;
  error?: string;
}

export interface ChangeFileView {
  agent: FileViewSide;
  worktree: FileViewSide;
}

export interface SyncOptions {
  repo: string;
  // Repo-relative files to sync instead of the whole repository — one Agent
  // Change Resolution rather than all of them. Requires `repo`.
  paths?: string[];
  dryRun: boolean;
  includeRisky: boolean;
  allowMaskedSync: boolean;
}

export interface TerminalSession {
  id: string;
  path: string;
  title: string;
  external?: boolean;
}

export interface TerminalSnapshot {
  id: string;
  data: string;
  seq: number;
  closed?: boolean;
  error?: string;
}

export interface RequestResult {
  repo: string;
  provider: string; // "github" | "gitlab" | ""
  method: string; // "api" | "browser" | "skipped"
  url: string;
  branch: string;
  target: string;
  error?: string;
}

export interface RequestResults {
  feature: string;
  items: RequestResult[] | null;
}

export interface RepoDiag {
  name: string;
  url: string;
  provider: string;
  tokenEnvName: string;
  tokenPresent: boolean;
  issues: string[] | null;
}

export interface GitDiag {
  issues: string[] | null;
  repos: RepoDiag[] | null;
}
