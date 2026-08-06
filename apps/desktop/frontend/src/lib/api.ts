// Typed wrappers around the Wails-injected bindings (window.go.main.App).
// Wails exposes each exported Go method on App as a Promise-returning function.
import type {
  BackupEntry,
  ChangeFileView,
  Config,
  DiffResult,
  SyncHistoryEntry,
  FeatureListResult,
  FeatureDeleteResult,
  FeatureCreateCheck,
  FeatureMetadata,
  FeaturePathsResult,
  RepoMeta,
  FeatureStatusResult,
  GitConfig,
  GitDiag,
  GitHubConfig,
  GitLabConfig,
  MaskFile,
  PrepareMetadata,
  PreviewResult,
  SecurityPreviewFile,
  CommitFileChange,
  CommitGraph,
  CommitMessageTemplateInfo,
  IntegrationPushResult,
  IntegrationReadiness,
  IntegrationResult,
  PushResult,
  RebasePreflight,
  UnpushedRepo,
  UnpushedResult,
  Repository,
  RepoRuntimeState,
  RequestResults,
  SecurityTemplate,
  SyncOptions,
  TerminalSession,
  TerminalSnapshot,
  WorkspaceEntry,
  WorkspacePathState,
  WorkspaceTreeNode,
  WorktreeTemplate,
  WorktreeTemplateTree,
} from "./types";

type AppBindings = {
  SelectWorkspaceDir(): Promise<string>;
  SelectProgram(): Promise<string>;
  OpenWorkspace(path: string): Promise<Config>;
  CopyText(text: string): Promise<void>;
  InitWorkspace(path: string, name: string): Promise<Config>;
  CurrentRoot(): Promise<string>;
  GetConfig(): Promise<Config>;
  ExportWorkspaceBundle(): Promise<string>;
  ImportWorkspaceBundle(): Promise<Config>;
  SelectWorkspaceBundleFile(): Promise<string>;
  SelectWorkspaceBundleTargetDir(): Promise<string>;
  ImportWorkspaceBundleFrom(zipPath: string, target: string): Promise<Config>;
  ListWorktreeTemplates(): Promise<WorktreeTemplate[]>;
  ListWorktreeTemplateTrees(): Promise<WorktreeTemplateTree[]>;
  ImportWorktreeTemplateFiles(): Promise<WorktreeTemplate[]>;
  ImportWorktreeTemplateFolder(): Promise<WorktreeTemplate>;
  ImportWorktreeTemplatePaths(paths: string[]): Promise<WorktreeTemplate[]>;
  UpdateWorktreeTemplate(t: WorktreeTemplate): Promise<void>;
  DeleteWorktreeTemplate(id: string): Promise<void>;
  ClearWorktreeTemplates(): Promise<void>;
  ReadWorktreeTemplateFile(id: string): Promise<string>;
  SaveWorktreeTemplateFile(id: string, content: string): Promise<void>;
  ReadWorktreeTemplateTreeFile(id: string, relPath: string): Promise<string>;
  SaveWorktreeTemplateTreeFile(
    id: string,
    relPath: string,
    content: string
  ): Promise<void>;
  OpenWorktreeTemplateFolder(): Promise<string>;
  ApplyWorkspaceRootTemplates(): Promise<void>;
  ApplyWorktreeTemplates(name: string): Promise<void>;
  ApplyAgentTemplates(name: string): Promise<void>;
  WorkspaceTree(path: string): Promise<WorkspaceTreeNode>;
  // Resolved per selected path, so browsing the tree runs no Git work.
  WorkspacePathState(path: string): Promise<WorkspacePathState>;
  // Registers an existing workspace path as a worktree template, inferring the
  // destination from where the source lives.
  RegisterWorktreeTemplateFromPath(path: string): Promise<WorktreeTemplate>;
  ReadWorkspaceFile(path: string): Promise<string>;
  SaveWorkspaceFile(path: string, content: string): Promise<void>;
  // Writes a modified workspace file back over its worktree-template source.
  OverwriteTemplateFromFile(path: string): Promise<void>;
  OpenPath(path: string): Promise<string>;
  OpenPathVSCode(path: string): Promise<string>;
  OpenPathInProgram(path: string, program: string): Promise<string>;
  DeleteWorkspacePath(path: string): Promise<void>;
  ListWorkspaces(): Promise<WorkspaceEntry[]>;
  RemoveWorkspace(path: string): Promise<void>;
  ListRepos(): Promise<Repository[]>;
  AddRepo(
    name: string,
    url: string,
    defaultBranch: string
  ): Promise<void>;
  RemoveRepo(name: string, deleteFiles: boolean): Promise<void>;
  Pull(): Promise<void>;
  PullRepo(name: string): Promise<void>;
  RepoLocalStates(): Promise<Record<string, boolean>>;
  RepoRuntimeStates(): Promise<RepoRuntimeState[]>;
  CheckoutRepoBranch(name: string, remoteBranch: string): Promise<void>;
  SetGitCredentials(host: string, username: string, secret: string): Promise<void>;
  ListFeatures(): Promise<FeatureListResult>;
  CheckFeatureCreation(name: string, base: string): Promise<FeatureCreateCheck>;
  CreateFeature(name: string, base: string, existingBranch: string): Promise<void>;
  FeatureRepoAdd(
    name: string,
    repoName: string,
    existingBranch: string,
    force: boolean
  ): Promise<RepoMeta>;
  FeatureRepoRecreate(
    name: string,
    repoName: string,
    existingBranch: string,
    force: boolean
  ): Promise<RepoMeta>;
  FeatureStatus(name: string): Promise<FeatureStatusResult>;
  // Integration operations on repo worktrees. `upstream` overrides the base
  // branch, which is how the commit graph integrates from a clicked ref.
  // A conflict is left in place as an Interrupted Integration, then finished with
  // ContinueIntegration or discarded with AbortIntegration (docs/adr/0002).
  RebaseFeature(
    name: string,
    repoFilter: string,
    upstream: string
  ): Promise<IntegrationResult>;
  MergeFeature(
    name: string,
    repoFilter: string,
    upstream: string
  ): Promise<IntegrationResult>;
  ContinueIntegration(
    name: string,
    repoFilter: string
  ): Promise<IntegrationResult>;
  AbortIntegration(name: string, repoFilter: string): Promise<IntegrationResult>;
  // Per-repository verdict for the confirm dialog. Costs filesystem work, so it
  // runs when the dialog opens rather than on every graph read.
  CheckIntegration(
    name: string,
    repoFilter: string
  ): Promise<IntegrationReadiness>;
  // One read per repository covers every feature branch: repo worktrees share
  // their main clone's object database.
  RepoCommitGraph(
    repoName: string,
    allBranches: boolean,
    limit: number,
    extraRefs: string[]
  ): Promise<CommitGraph>;
  CommitFileChanges(
    repoName: string,
    sha: string
  ): Promise<CommitFileChange[]>;
  // force uses --force-with-lease against the SHA origin/<branch> is read to
  // hold. The per-repository result must be inspected: a repository that failed
  // is not a thrown error.
  PushFeature(
    name: string,
    repoFilter: string,
    force: boolean
  ): Promise<PushResult>;
  // What a push would send, per repository, with the resolved range.
  UnpushedCommits(
    name: string,
    repoFilter: string,
    limit: number
  ): Promise<UnpushedResult>;
  // The commits one repo worktree is sitting on, scoped to its HEAD. Use
  // RepoCommitGraph for the repository-wide view.
  WorktreeCommits(
    name: string,
    repoName: string,
    limit: number
  ): Promise<UnpushedRepo>;
  // Read-only inspection for the rebase dialog. Costs several git subprocesses
  // per repository, so it runs when the dialog opens rather than on every load.
  RebasePreflight(name: string, repoFilter: string): Promise<RebasePreflight>;
  // Rebase, then force-push the repositories the rebase actually rewrote.
  // Which those are is decided in internal/feature, so this cannot drift from
  // the CLI (docs/adr/0003).
  RebaseAndPush(
    name: string,
    repoFilter: string,
    upstream: string,
    push: boolean
  ): Promise<IntegrationPushResult>;
  // The workspace's Commit Message Template and what it currently renders to.
  // An empty feature name previews with stand-in values, for the settings page.
  CommitMessageTemplateInfo(
    featureName: string
  ): Promise<CommitMessageTemplateInfo>;
  FeatureDelete(
    name: string,
    deleteBranch: boolean,
    force: boolean
  ): Promise<FeatureDeleteResult>;
  LoadFeature(name: string): Promise<FeatureMetadata>;
  FeaturePaths(name: string): Promise<FeaturePathsResult>;
  AgentPrepare(name: string, backup: boolean): Promise<PrepareMetadata>;
  AgentPrepareRepo(
    name: string,
    repoName: string,
    backup: boolean
  ): Promise<PrepareMetadata>;
  AgentDiff(name: string, repoFilter: string): Promise<DiffResult>;
  AgentChangeFileView(
    name: string,
    repoName: string,
    path: string
  ): Promise<ChangeFileView>;
  // One Repo Worktree Change: the file as the branch's last commit has it,
  // against the file on disk now — what the next commit would record.
  WorktreeChangeFileView(
    name: string,
    repoName: string,
    path: string
  ): Promise<ChangeFileView>;
  AgentSync(name: string, opt: SyncOptions): Promise<void>;
  SyncAndCommit(name: string, message: string, opt: SyncOptions): Promise<void>;
  SyncCommitPush(name: string, message: string, opt: SyncOptions): Promise<void>;
  AgentRestoreFromWorktree(name: string, repoName: string, path: string): Promise<void>;
  AgentRestoreRepoFromWorktree(name: string, repoName: string): Promise<number>;
  AgentDelete(name: string): Promise<void>;
  AllSyncHistory(): Promise<SyncHistoryEntry[]>;
  RollbackSync(name: string, repo: string, id: string): Promise<void>;
  SyncHistoryCounts(name: string): Promise<Record<string, number>>;
  GetAgentIgnore(): Promise<string>;
  SaveAgentIgnore(content: string): Promise<void>;
  GetMaskFile(): Promise<MaskFile>;
  SaveMaskFile(m: MaskFile): Promise<void>;
  ListSecurityTemplates(): Promise<SecurityTemplate[]>;
  ApplySecurityTemplates(keys: string[], replace: boolean): Promise<void>;
  ExportAgentSecurity(): Promise<string>;
  ImportAgentSecurity(): Promise<string>;
  ScanSecurityPreview(repoName: string): Promise<PreviewResult>;
  ScanSecurityPreviewFile(
    repoName: string,
    path: string
  ): Promise<SecurityPreviewFile>;
  ListBackups(): Promise<BackupEntry[]>;
  DeleteBackup(path: string): Promise<void>;
  DeleteAllBackups(): Promise<number>;
  RestoreBackup(path: string): Promise<void>;
  Commit(name: string, message: string, repoFilter: string): Promise<void>;
  Push(name: string, repoFilter: string): Promise<PushResult>;
  CreateMergeRequests(name: string, title: string): Promise<RequestResults>;
  OpenURL(url: string): Promise<void>;
  SaveGitSettings(
    git: GitConfig,
    gitlab: GitLabConfig,
    github: GitHubConfig
  ): Promise<void>;
  DiagnoseGit(): Promise<GitDiag>;
  OpenWorkspaceFolder(): Promise<string>;
  OpenWorkspaceTerminal(): Promise<string>;
  OpenWorkspaceVSCode(): Promise<string>;
  OpenInEditor(name: string, editor: string): Promise<string>;
  OpenInTerminal(name: string): Promise<string>;
  OpenAgentCommandTerminal(
    name: string,
    terminalProgram: string,
    command: string
  ): Promise<string>;
  OpenFeatureFolder(name: string): Promise<string>;
  OpenRepoFolder(name: string, repo: string): Promise<string>;
  OpenRepoInProgram(name: string, repo: string, program: string): Promise<string>;
  TerminalOpen(path: string): Promise<TerminalSession>;
  TerminalOpenWithProgram(
    path: string,
    terminalProgram: string
  ): Promise<TerminalSession>;
  TerminalOpenFeatureAgent(
    name: string,
    terminalProgram: string
  ): Promise<TerminalSession>;
  AgentRun(name: string, command: string): Promise<TerminalSession>;
  AgentRunWithProgram(
    name: string,
    command: string,
    terminalProgram: string
  ): Promise<TerminalSession>;
  TerminalWrite(id: string, data: string): Promise<void>;
  TerminalResize(id: string, cols: number, rows: number): Promise<void>;
  TerminalSnapshot(id: string): Promise<TerminalSnapshot>;
  TerminalClose(id: string): Promise<void>;
  SetLogLevel(level: string): Promise<void>;
  LogLevel(): Promise<string>;
  LogFilePath(): Promise<string>;
  OpenLogFile(): Promise<void>;
  OpenLogFolder(): Promise<void>;
};

declare global {
  interface Window {
    go?: { main?: { App?: AppBindings } };
  }
}

function bindings(): AppBindings {
  const app = window.go?.main?.App;
  if (!app) {
    throw new Error(
      "Wails bindings are not available. Run the app via `wails dev` or a built binary."
    );
  }
  return app;
}

export const api: AppBindings = new Proxy({} as AppBindings, {
  get(_target, prop: string) {
    return (...args: unknown[]) =>
      (bindings() as unknown as Record<string, (...a: unknown[]) => unknown>)[
        prop
      ](...args);
  },
});

// Normalizes the error thrown by a binding call into a string message.
export function errMessage(e: unknown): string {
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  return String(e);
}
