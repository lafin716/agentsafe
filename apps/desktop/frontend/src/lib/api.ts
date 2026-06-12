// Typed wrappers around the Wails-injected bindings (window.go.main.App).
// Wails exposes each exported Go method on App as a Promise-returning function.
import type {
  BackupEntry,
  Config,
  DiffResult,
  SyncHistoryEntry,
  FeatureListResult,
  FeatureDeleteResult,
  FeatureMetadata,
  RepoMeta,
  FeatureStatusResult,
  GitConfig,
  GitDiag,
  GitHubConfig,
  GitLabConfig,
  MaskFile,
  PrepareMetadata,
  RebaseResult,
  Repository,
  RequestResults,
  SecurityTemplate,
  SyncOptions,
  WorkspaceEntry,
} from "./types";

type AppBindings = {
  SelectWorkspaceDir(): Promise<string>;
  SelectProgram(): Promise<string>;
  OpenWorkspace(path: string): Promise<Config>;
  InitWorkspace(path: string, name: string): Promise<Config>;
  CurrentRoot(): Promise<string>;
  GetConfig(): Promise<Config>;
  ListWorkspaces(): Promise<WorkspaceEntry[]>;
  RemoveWorkspace(path: string): Promise<void>;
  ListRepos(): Promise<Repository[]>;
  AddRepo(
    name: string,
    url: string,
    typ: string,
    defaultBranch: string
  ): Promise<void>;
  RemoveRepo(name: string): Promise<void>;
  Pull(): Promise<void>;
  ListFeatures(): Promise<FeatureListResult>;
  CreateFeature(name: string, base: string, existingBranch: string): Promise<void>;
  FeatureRepoAdd(
    name: string,
    repoName: string,
    existingBranch: string
  ): Promise<RepoMeta>;
  FeatureRepoRecreate(
    name: string,
    repoName: string,
    existingBranch: string,
    force: boolean
  ): Promise<RepoMeta>;
  FeatureStatus(name: string): Promise<FeatureStatusResult>;
  RebaseFeature(name: string, repoFilter: string): Promise<RebaseResult>;
  FeatureDelete(
    name: string,
    deleteBranch: boolean,
    force: boolean
  ): Promise<FeatureDeleteResult>;
  LoadFeature(name: string): Promise<FeatureMetadata>;
  AgentPrepare(name: string, backup: boolean): Promise<PrepareMetadata>;
  AgentDiff(name: string, repoFilter: string): Promise<DiffResult>;
  AgentSync(name: string, opt: SyncOptions): Promise<void>;
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
  ListBackups(): Promise<BackupEntry[]>;
  DeleteBackup(path: string): Promise<void>;
  DeleteAllBackups(): Promise<number>;
  RestoreBackup(path: string): Promise<void>;
  Commit(name: string, message: string, repoFilter: string): Promise<void>;
  Push(name: string, repoFilter: string): Promise<void>;
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
