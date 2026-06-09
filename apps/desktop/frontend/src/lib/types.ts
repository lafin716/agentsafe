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
  Type: string;
  TestCommand: string;
}

export interface AgentConfig {
  IgnoreFileName: string;
  MaskFileName: string;
  DefaultExclude: string[] | null;
}

export interface GitLabConfig {
  BaseURL: string;
  TokenEnv: string;
  TargetBranch: string;
}

export interface GitHubConfig {
  BaseURL: string;
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

export interface RepoStatus {
  name: string;
  status: string;
}

export interface FeatureStatusResult {
  feature: string;
  branch: string;
  agentReady?: boolean;
  repositories: RepoStatus[] | null;
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

export interface RepoMeta {
  name: string;
  worktreePath: string;
  branch: string;
  baseBranch: string;
}

export interface FeatureMetadata {
  name: string;
  branch: string;
  baseBranch: string;
  createdAt: string;
  repositories: RepoMeta[] | null;
}

export interface PrepareRepo {
  name: string;
  source: string;
  agent: string;
  copiedFiles: number;
  ignoredFiles: number;
  maskedFiles: string[] | null;
  preparedHashes?: Record<string, string>;
}

export interface PrepareMetadata {
  feature: string;
  preparedAt: string;
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
