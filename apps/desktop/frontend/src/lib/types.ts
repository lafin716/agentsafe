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

export interface WorkspaceTreeNode {
  name: string;
  path: string;
  relPath: string;
  isDir: boolean;
  size: number;
  modTime: string;
  featureName?: string;
  branch?: string;
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

export type RebaseStatus =
  | "rebased"
  | "up-to-date"
  | "skipped"
  | "failed"
  | string;

export interface RebaseRepoResult {
  name: string;
  branch: string;
  baseBranch: string;
  status: RebaseStatus;
  detail: string;
}

export interface RebaseResult {
  feature: string;
  repositories: RebaseRepoResult[] | null;
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

export interface SyncOptions {
  repo: string;
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
