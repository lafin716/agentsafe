import { useEffect, useState } from "react";
import { Bug, Download, FolderOpen, GripVertical, Languages, Pencil, Plus, Save, Stethoscope, Terminal, Trash2, Upload, Wrench, X } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { LOCALES, type Locale } from "@/i18n/translations";
import { api, errMessage } from "@/lib/api";
import type { Config, GitDiag } from "@/lib/types";
import { cn } from "@/lib/utils";
import {
  addToolEntry,
  deleteToolEntry,
  reorderToolEntries,
  setDefaultToolId,
  toolLabel,
  type ToolEntry,
  updateToolEntry,
  useToolSettings,
} from "@/lib/tool";

interface Props {
  config: Config | null;
  onChanged: () => void | Promise<void>;
}

export function SettingsPage({ config, onChanged }: Props) {
  const { locale, setLocale, t } = useI18n();
  const { notify } = useToast();
  const [terminalProgram, setTerminalProgram] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.terminalProgram") || "powershell";
    } catch {
      return "powershell";
    }
  });
  const [devMode, setDevMode] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.devMode") === "true";
    } catch {
      return false;
    }
  });

  function choose(l: Locale) {
    if (l === locale) return;
    setLocale(l);
    notify(t("settings.saved"), "success");
  }

  function changeTerminalProgram(v: string) {
    setTerminalProgram(v);
    try {
      localStorage.setItem("agentsafe.terminalProgram", v);
    } catch {
      /* ignore */
    }
    notify(t("settings.saved"), "success");
  }

  async function changeDevMode(on: boolean) {
    setDevMode(on);
    try {
      localStorage.setItem("agentsafe.devMode", String(on));
    } catch {
      /* ignore */
    }
    try {
      // Raise verbosity to debug while developer mode is on; baseline info otherwise.
      await api.SetLogLevel(on ? "debug" : "info");
    } catch (e) {
      notify(errMessage(e), "error");
      return;
    }
    notify(t("settings.saved"), "success");
  }

  return (
    <div className="max-w-2xl space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Languages className="size-5" /> {t("settings.languageTitle")}
          </CardTitle>
          <CardDescription>{t("settings.languageDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="inline-flex rounded-lg border bg-card p-1">
            {LOCALES.map((l) => (
              <button
                key={l.id}
                onClick={() => choose(l.id)}
                className={cn(
                  "rounded-md px-4 py-1.5 text-sm transition-colors",
                  l.id === locale
                    ? "bg-secondary font-medium text-secondary-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {l.label}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>

      <WorkspaceTransfer
        config={config}
        onChanged={onChanged}
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="size-5" /> Default terminal
          </CardTitle>
          <CardDescription>
            Choose the shell used by embedded terminal tabs across the app.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-1.5">
            <Label htmlFor="defaultTerminalProgram">Terminal program</Label>
            <select
              id="defaultTerminalProgram"
              value={terminalProgram}
              onChange={(e) => changeTerminalProgram(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              <option value="powershell">PowerShell</option>
              <option value="pwsh">PowerShell 7</option>
              <option value="cmd">Command Prompt</option>
              <option value="git-bash">Git Bash</option>
              <option value="wt">Windows Terminal</option>
              <option value="default">System default</option>
            </select>
          </div>
        </CardContent>
      </Card>

      <ToolSettingsCard />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Bug className="size-5" /> {t("settings.devModeTitle")}
          </CardTitle>
          <CardDescription>{t("settings.devModeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <label className="flex items-center gap-3 text-sm">
            <input
              type="checkbox"
              checked={devMode}
              onChange={(e) => changeDevMode(e.target.checked)}
              className="size-4"
            />
            {t("settings.devModeLabel")}
          </label>
        </CardContent>
      </Card>

      {config ? (
        <GitSettings config={config} onChanged={onChanged} />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>{t("settings.gitTitle")}</CardTitle>
            <CardDescription>{t("settings.gitNoWorkspace")}</CardDescription>
          </CardHeader>
        </Card>
      )}
    </div>
  );
}

function ToolSettingsCard() {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const settings = useToolSettings();
  const [dialogEntry, setDialogEntry] = useState<ToolEntry | null | undefined>();
  const [draggingId, setDraggingId] = useState<string | null>(null);

  function changeDefault(id: string) {
    setDefaultToolId(id);
    notify(t("settings.saved"), "success");
  }

  function dropOn(targetId: string) {
    if (!draggingId || draggingId === targetId) return;
    const ids = settings.tools.map((entry) => entry.id);
    const from = ids.indexOf(draggingId);
    const to = ids.indexOf(targetId);
    if (from < 0 || to < 0) return;
    ids.splice(from, 1);
    ids.splice(to, 0, draggingId);
    reorderToolEntries(ids);
    setDraggingId(null);
    notify(t("settings.saved"), "success");
  }

  async function remove(entry: ToolEntry) {
    const ok = await confirm({
      message: t("settings.toolDeleteConfirm", { label: entry.label }),
      danger: true,
    });
    if (!ok) return;
    try {
      deleteToolEntry(entry.id);
      notify(t("settings.saved"), "success");
    } catch (e) {
      notify(toolSettingsError(t, e), "error");
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wrench className="size-5" /> {t("settings.defaultToolTitle")}
          </CardTitle>
          <CardDescription>{t("settings.defaultToolDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-1.5">
            <Label htmlFor="defaultTool">{t("settings.defaultToolTitle")}</Label>
            <select
              id="defaultTool"
              value={settings.defaultToolId}
              onChange={(event) => changeDefault(event.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              {settings.tools.map((entry) => (
                <option key={entry.id} value={entry.id}>
                  {entry.label}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <Label>{t("settings.registeredTools")}</Label>
              <Button size="sm" variant="outline" onClick={() => setDialogEntry(null)}>
                <Plus className="size-4" /> {t("settings.toolAdd")}
              </Button>
            </div>
            <div className="space-y-2">
              {settings.tools.map((entry) => (
                <div
                  key={entry.id}
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => dropOn(entry.id)}
                  className="flex items-center gap-2 rounded-md border bg-card p-2"
                >
                  <button
                    type="button"
                    draggable
                    onDragStart={(event) => {
                      event.dataTransfer.effectAllowed = "move";
                      setDraggingId(entry.id);
                    }}
                    onDragEnd={() => setDraggingId(null)}
                    title={t("settings.toolDrag")}
                    aria-label={t("settings.toolDrag")}
                    className="cursor-grab rounded p-1 text-muted-foreground hover:bg-accent active:cursor-grabbing"
                  >
                    <GripVertical className="size-4" />
                  </button>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{entry.label}</div>
                    <div className="truncate font-mono text-xs text-muted-foreground">
                      {entry.command}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("settings.toolEdit")}
                    onClick={() => setDialogEntry(entry)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8 text-muted-foreground hover:text-destructive"
                    title={t("common.delete")}
                    disabled={settings.tools.length === 1}
                    onClick={() => void remove(entry)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {dialogEntry !== undefined && (
        <ToolEntryDialog
          entry={dialogEntry}
          onClose={() => setDialogEntry(undefined)}
        />
      )}
    </>
  );
}

function ToolEntryDialog({
  entry,
  onClose,
}: {
  entry: ToolEntry | null;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const { notify } = useToast();
  const [label, setLabel] = useState(entry?.label ?? "");
  const [command, setCommand] = useState(entry?.command ?? "");
  const [error, setError] = useState("");

  async function pickProgram() {
    try {
      const selected = await api.SelectProgram();
      if (!selected) return;
      setCommand(selected);
      if (!label.trim()) setLabel(toolLabel(selected));
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  function save(event: React.FormEvent) {
    event.preventDefault();
    try {
      if (entry) updateToolEntry(entry.id, label, command);
      else addToolEntry(label, command);
      notify(t("settings.saved"), "success");
      onClose();
    } catch (e) {
      setError(toolSettingsError(t, e));
    }
  }

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="toolEntryDialogTitle"
        className="w-full max-w-lg rounded-lg border bg-card p-5 shadow-xl"
        onClick={(event) => event.stopPropagation()}
        onSubmit={save}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id="toolEntryDialogTitle" className="text-base font-semibold">
              {t(entry ? "settings.toolEdit" : "settings.toolAdd")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("settings.toolDialogDesc")}
            </p>
          </div>
          <Button type="button" variant="ghost" size="icon" className="size-8" onClick={onClose}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="mt-4 space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="toolLabel">{t("settings.toolLabel")}</Label>
            <Input id="toolLabel" autoFocus value={label} onChange={(event) => setLabel(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="toolCommand">{t("settings.toolCommand")}</Label>
            <div className="flex gap-2">
              <Input
                id="toolCommand"
                value={command}
                onChange={(event) => setCommand(event.target.value)}
                placeholder="code"
              />
              <Button type="button" variant="outline" onClick={() => void pickProgram()}>
                <FolderOpen className="size-4" /> {t("settings.toolChooseFile")}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">{t("settings.toolCommandHint")}</p>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit">{t("common.save")}</Button>
        </div>
      </form>
    </div>
  );
}

function toolSettingsError(
  t: (key: string, vars?: Record<string, string | number>) => string,
  error: unknown
): string {
  const code = errMessage(error);
  const known = new Set([
    "requiredLabel",
    "requiredCommand",
    "invalidCommand",
    "duplicateLabel",
    "duplicateCommand",
    "lastTool",
  ]);
  return known.has(code) ? t(`settings.toolError.${code}`) : code;
}

function WorkspaceTransfer({
  config,
  onChanged,
}: {
  config: Config | null;
  onChanged: () => void | Promise<void>;
}) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [busy, setBusy] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [bundlePath, setBundlePath] = useState("");
  const [targetDir, setTargetDir] = useState("");

  async function exportBundle() {
    try {
      setBusy(true);
      const path = await api.ExportWorkspaceBundle();
      if (path) notify(t("settings.transferExported", { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function importBundle() {
    try {
      setBusy(true);
      const cfg = await api.ImportWorkspaceBundleFrom(bundlePath, targetDir);
      await onChanged();
      notify(t("settings.transferImported", { name: cfg.Workspace.Name }), "success");
      setImportOpen(false);
      setBundlePath("");
      setTargetDir("");
      const clone = await confirm({
        title: t("settings.transferCloneTitle"),
        message: t("settings.transferCloneConfirm"),
      });
      if (clone) {
        await api.Pull();
        notify(t("toast.pullCompleted"), "success");
        await onChanged();
      }
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function chooseBundleFile() {
    try {
      const path = await api.SelectWorkspaceBundleFile();
      if (path) setBundlePath(path);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function chooseTargetDir() {
    try {
      const path = await api.SelectWorkspaceBundleTargetDir();
      if (path) setTargetDir(path);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.transferTitle")}</CardTitle>
        <CardDescription>{t("settings.transferDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={exportBundle} disabled={busy || !config}>
          <Download className="size-4" /> {t("settings.transferExport")}
        </Button>
        <Button onClick={() => setImportOpen(true)} disabled={busy}>
          <Upload className="size-4" /> {t("settings.transferImport")}
        </Button>
      </CardContent>
      {importOpen && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4"
          onClick={() => setImportOpen(false)}
        >
          <div
            className="w-full max-w-xl rounded-lg border bg-card p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-base font-semibold">
              {t("settings.transferImportTitle")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("settings.transferImportDesc")}
            </p>
            <div className="mt-4 space-y-4">
              <div className="space-y-1.5">
                <Label>{t("settings.transferBundleFile")}</Label>
                <div className="flex gap-2">
                  <Input
                    value={bundlePath || t("settings.transferNoFile")}
                    readOnly
                    className={!bundlePath ? "text-muted-foreground" : ""}
                  />
                  <Button variant="outline" onClick={chooseBundleFile} disabled={busy}>
                    <Upload className="size-4" /> {t("settings.transferChooseFile")}
                  </Button>
                </div>
              </div>
              <div className="space-y-1.5">
                <Label>{t("settings.transferTargetFolder")}</Label>
                <div className="flex gap-2">
                  <Input
                    value={targetDir || t("settings.transferNoFolder")}
                    readOnly
                    className={!targetDir ? "text-muted-foreground" : ""}
                  />
                  <Button variant="outline" onClick={chooseTargetDir} disabled={busy}>
                    <FolderOpen className="size-4" /> {t("settings.transferChooseFolder")}
                  </Button>
                </div>
              </div>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <Button variant="outline" onClick={() => setImportOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                onClick={importBundle}
                disabled={busy || !bundlePath || !targetDir}
              >
                {t("settings.transferRunImport")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Card>
  );
}

function GitSettings({
  config,
  onChanged,
}: {
  config: Config;
  onChanged: () => void | Promise<void>;
}) {
  const { t } = useI18n();
  const { notify } = useToast();
  const [busy, setBusy] = useState(false);
  const [diag, setDiag] = useState<GitDiag | null>(null);

  // Git
  const [baseBranch, setBaseBranch] = useState("");
  const [branchPrefix, setBranchPrefix] = useState("");
  // GitLab
  const [glUrl, setGlUrl] = useState("");
  const [glToken, setGlToken] = useState("");
  const [glTarget, setGlTarget] = useState("");
  // GitHub
  const [ghToken, setGhToken] = useState("");
  const [ghTarget, setGhTarget] = useState("");

  useEffect(() => {
    setBaseBranch(config.Git?.DefaultBaseBranch ?? "");
    setBranchPrefix(config.Git?.BranchPrefix ?? "");
    setGlUrl(config.GitLab?.BaseURL ?? "");
    setGlToken(config.GitLab?.TokenEnv ?? "");
    setGlTarget(config.GitLab?.TargetBranch ?? "");
    setGhToken(config.GitHub?.TokenEnv ?? "");
    setGhTarget(config.GitHub?.TargetBranch ?? "");
  }, [config]);

  async function save() {
    try {
      setBusy(true);
      await api.SaveGitSettings(
        { DefaultBaseBranch: baseBranch.trim(), BranchPrefix: branchPrefix.trim() },
        {
          BaseURL: glUrl.trim(),
          TokenEnv: glToken.trim(),
          TargetBranch: glTarget.trim(),
        },
        {
          TokenEnv: ghToken.trim(),
          TargetBranch: ghTarget.trim(),
        }
      );
      await onChanged();
      notify(t("toast.gitSettingsSaved"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function diagnose() {
    try {
      setBusy(true);
      setDiag(await api.DiagnoseGit());
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  // Lightweight inline validation hints.
  const prefixHint =
    branchPrefix && !branchPrefix.endsWith("/")
      ? t("settings.hintPrefixSlash")
      : "";
  const urlHint = (v: string) =>
    v && !/^https?:\/\//.test(v) ? t("settings.hintUrlScheme") : "";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.gitTitle")}</CardTitle>
        <CardDescription>{t("settings.gitDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <Section title={t("settings.gitSection")}>
          <Field label={t("workspace.baseBranch")}>
            <Input value={baseBranch} onChange={(e) => setBaseBranch(e.target.value)} placeholder="develop" />
          </Field>
          <Field label={t("workspace.branchPrefix")} hint={prefixHint}>
            <Input value={branchPrefix} onChange={(e) => setBranchPrefix(e.target.value)} placeholder="feature/" />
          </Field>
        </Section>

        <Section title="GitHub">
          <Field label={t("settings.tokenEnv")}>
            <Input value={ghToken} onChange={(e) => setGhToken(e.target.value)} placeholder="GITHUB_TOKEN" />
          </Field>
          <Field label={t("settings.targetBranch")}>
            <Input value={ghTarget} onChange={(e) => setGhTarget(e.target.value)} placeholder="main" />
          </Field>
        </Section>

        <Section title="GitLab">
          <Field label={t("settings.baseUrl")} hint={urlHint(glUrl)}>
            <Input value={glUrl} onChange={(e) => setGlUrl(e.target.value)} placeholder="https://gitlab.example.com" />
          </Field>
          <Field label={t("settings.tokenEnv")}>
            <Input value={glToken} onChange={(e) => setGlToken(e.target.value)} placeholder="GITLAB_TOKEN" />
          </Field>
          <Field label={t("settings.targetBranch")}>
            <Input value={glTarget} onChange={(e) => setGlTarget(e.target.value)} placeholder="develop" />
          </Field>
        </Section>

        <div className="flex gap-2">
          <Button onClick={save} disabled={busy}>
            <Save className="size-4" /> {t("settings.save")}
          </Button>
          <Button variant="outline" onClick={diagnose} disabled={busy}>
            <Stethoscope className="size-4" /> {t("settings.diagnose")}
          </Button>
        </div>

        {diag && <Diagnostics diag={diag} />}
      </CardContent>
    </Card>
  );
}

function Diagnostics({ diag }: { diag: GitDiag }) {
  const { t } = useI18n();
  const issues = diag.issues ?? [];
  const repos = diag.repos ?? [];
  return (
    <div className="space-y-3 rounded-md border bg-muted/30 p-3 text-sm">
      <div className="font-medium">{t("settings.diagTitle")}</div>
      {issues.length > 0 && (
        <ul className="list-disc space-y-0.5 pl-5 text-amber-600">
          {issues.map((i, n) => (
            <li key={n}>{i}</li>
          ))}
        </ul>
      )}
      {repos.length === 0 ? (
        <p className="text-muted-foreground">{t("settings.diagNoRepos")}</p>
      ) : (
        <ul className="divide-y rounded-md border bg-background">
          {repos.map((r) => (
            <li key={r.name} className="px-3 py-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{r.name}</span>
                <Badge variant="outline">
                  {r.provider === "github"
                    ? "GitHub"
                    : r.provider === "gitlab"
                      ? "GitLab"
                      : t("request.providerUnknown")}
                </Badge>
                {r.provider && (
                  <Badge variant={r.tokenPresent ? "success" : "warning"}>
                    {r.tokenEnvName}{" "}
                    {r.tokenPresent
                      ? t("settings.diagTokenPresent")
                      : t("settings.diagTokenMissing")}
                  </Badge>
                )}
              </div>
              {(r.issues ?? []).length > 0 && (
                <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-amber-600">
                  {(r.issues ?? []).map((i, n) => (
                    <li key={n}>{i}</li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-3">
      <div className="text-sm font-semibold text-muted-foreground">{title}</div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">{children}</div>
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-xs text-amber-600">{hint}</p>}
    </div>
  );
}
