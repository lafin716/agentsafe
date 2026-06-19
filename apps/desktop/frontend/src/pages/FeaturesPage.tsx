import {
  Fragment,
  useCallback,
  useEffect,
  useState,
  type Dispatch,
  type PointerEvent as ReactPointerEvent,
  type SetStateAction,
} from "react";
import {
  AlertTriangle,
  ChevronUp,
  Code2,
  FolderOpen,
  Loader2,
  Plus,
  RefreshCw,
  Terminal,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm";
import type { FeatureCreateCheck, FeatureDeleteResult, FeatureEntry, TerminalSession } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { useI18n } from "@/i18n/I18nProvider";
import { TerminalPanel } from "@/components/TerminalPanel";

interface Props {
  onOpen: (name: string) => void;
  // Terminal sessions are shared (per feature) with the feature detail page so
  // opening/closing in either view stays in sync and survives navigation.
  terminalTabs: Record<string, TerminalSession[]>;
  setTerminalTabs: Dispatch<SetStateAction<Record<string, TerminalSession[]>>>;
  // Inline-panel UI state, lifted to App so the panel reappears as left.
  expanded: Set<string>;
  setExpanded: Dispatch<SetStateAction<Set<string>>>;
  activeTerminal: Record<string, string>;
  setActiveTerminal: Dispatch<SetStateAction<Record<string, string>>>;
  heights: Record<string, number>;
  setHeights: Dispatch<SetStateAction<Record<string, number>>>;
}

function defaultTerminalProgram(): string {
  try {
    return localStorage.getItem("agentsafe.terminalProgram") || "powershell";
  } catch {
    return "powershell";
  }
}

const DEFAULT_TERMINAL_HEIGHT = 320;
const MIN_TERMINAL_HEIGHT = 160;
const MAX_TERMINAL_HEIGHT = 900;

export function FeaturesPage({
  onOpen,
  terminalTabs,
  setTerminalTabs,
  expanded,
  setExpanded,
  activeTerminal,
  setActiveTerminal,
  heights,
  setHeights,
}: Props) {
  const { notify } = useToast();
  const { t } = useI18n();
  const confirm = useConfirm();
  const [features, setFeatures] = useState<FeatureEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [createCheck, setCreateCheck] = useState<FeatureCreateCheck | null>(null);
  const [existingBranch, setExistingBranch] = useState<"reuse" | "recreate">("reuse");
  const [syncing, setSyncing] = useState<Record<string, boolean>>({});
  const [syncErrors, setSyncErrors] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    try {
      const res = await api.ListFeatures();
      setFeatures(res.features ?? []);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  // openNewTerminal starts an additional pty session for the feature, appends it
  // as a tab in the shared per-feature list, makes it active, and expands the panel.
  async function openNewTerminal(name: string) {
    try {
      setBusy(true);
      const session = await api.TerminalOpenFeatureAgent(name, defaultTerminalProgram());
      // External terminals open an OS window with no embeddable pty; don't add a tab.
      if (session.external) {
        notify(t("toast.openedPath", { path: session.path }), "success");
        return;
      }
      const tab: TerminalSession = { ...session, title: `Terminal · ${name}` };
      setTerminalTabs((prev) => {
        const current = prev[name] ?? [];
        if (current.some((t) => t.id === tab.id)) return prev;
        return { ...prev, [name]: [...current, tab] };
      });
      setActiveTerminal((prev) => ({ ...prev, [name]: tab.id }));
      setExpanded((prev) => {
        const next = new Set(prev);
        next.add(name);
        return next;
      });
      notify(t("toast.openedEmbeddedTerminal", { path: session.path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  // togglePanel shows/hides the inline panel (keeping pty sessions alive). When no
  // terminal exists yet it opens the first one.
  function togglePanel(name: string) {
    if ((terminalTabs[name] ?? []).length === 0) {
      void openNewTerminal(name);
      return;
    }
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  // collapsePanel hides the panel but keeps the pty sessions alive, so re-opening
  // restores the same session output (via TerminalPanel's snapshot replay).
  function collapsePanel(name: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      next.delete(name);
      return next;
    });
  }

  async function closeTerminal(name: string, id: string) {
    try {
      await api.TerminalClose(id);
    } catch {
      /* terminal may already be closed */
    }
    const remaining = (terminalTabs[name] ?? []).filter((tab) => tab.id !== id);
    setTerminalTabs((prev) => ({
      ...prev,
      [name]: (prev[name] ?? []).filter((tab) => tab.id !== id),
    }));
    setActiveTerminal((prev) => {
      if (prev[name] !== id) return prev;
      const next = { ...prev };
      if (remaining.length > 0) next[name] = remaining[remaining.length - 1].id;
      else delete next[name];
      return next;
    });
    if (remaining.length === 0) collapsePanel(name);
  }

  async function syncFeature(name: string) {
    try {
      setSyncing((prev) => ({ ...prev, [name]: true }));
      setSyncErrors((prev) => {
        const next = { ...prev };
        delete next[name];
        return next;
      });
      await api.AgentSync(name, {
        repo: "",
        dryRun: false,
        includeRisky: false,
        allowMaskedSync: false,
      });
      notify(t("toast.syncCompleted"), "success");
      await load();
    } catch (e) {
      const message = errMessage(e);
      const blocked = /risky|masked|blocked|include-risky|allow-masked/i.test(message);
      const nextMessage = blocked
        ? `${t("features.syncBlockedDetail")}\n${message}`
        : message;
      setSyncErrors((prev) => ({ ...prev, [name]: nextMessage }));
      notify(nextMessage, "error");
    } finally {
      setSyncing((prev) => ({ ...prev, [name]: false }));
    }
  }

  // startResize drags the terminal panel taller/shorter. TerminalPanel's internal
  // ResizeObserver picks up the height change and refits xterm automatically.
  function startResize(name: string, e: ReactPointerEvent) {
    e.preventDefault();
    const startY = e.clientY;
    const startHeight = heights[name] ?? DEFAULT_TERMINAL_HEIGHT;
    const onMove = (ev: PointerEvent) => {
      const next = Math.min(
        MAX_TERMINAL_HEIGHT,
        Math.max(MIN_TERMINAL_HEIGHT, startHeight + (ev.clientY - startY))
      );
      setHeights((prev) => ({ ...prev, [name]: next }));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }

  async function openVSCode(name: string) {
    try {
      await api.OpenInEditor(name, "code");
      notify(t("toast.openedVSCode", { name }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function openFolder(name: string) {
    try {
      const p = await api.OpenFeatureFolder(name);
      notify(t("toast.openedPath", { path: p }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function deleteFeature(name: string) {
    if (!(await confirm({ message: t("feature.deleteConfirm", { name }), danger: true })))
      return;
    try {
      setBusy(true);
      let result: FeatureDeleteResult | undefined;
      try {
        result = await api.FeatureDelete(name, false, false);
      } catch (e) {
        const msg = errMessage(e);
        // Offer a force delete when the worktree has uncommitted changes.
        if (/uncommitted|changes/i.test(msg)) {
          if (
            !(await confirm({
              message: t("feature.deleteForceConfirm"),
              danger: true,
            }))
          )
            return;
          result = await api.FeatureDelete(name, false, true);
        } else {
          throw e;
        }
      }
      for (const warning of result?.warnings ?? []) {
        notify(warning, "error");
      }
      notify(t("toast.featureDeleted"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    const featureName = name.trim();
    try {
      setBusy(true);
      const check = await api.CheckFeatureCreation(featureName, "");
      if (check.hasConflicts || check.blocked) {
        setExistingBranch("reuse");
        setCreateCheck(check);
      } else {
        await completeCreate(featureName, "error");
      }
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function completeCreate(
    featureName: string,
    policy: "error" | "reuse" | "recreate"
  ) {
    await api.CreateFeature(featureName, "", policy);
    notify(t("toast.featureCreated", { name: featureName }), "success");
    setName("");
    setCreateCheck(null);
    setExistingBranch("reuse");
    await load();
    if (
      await confirm({
        title: t("features.prepareAgentTitle"),
        message: t("features.prepareAgentConfirm", { name: featureName }),
        confirmLabel: t("features.prepareAgentYes"),
      })
    ) {
      const meta = await api.AgentPrepare(featureName, true);
      const copied = (meta.repositories ?? []).reduce((n, r) => n + r.copiedFiles, 0);
      notify(t("toast.agentPrepared", { count: copied }), "success");
    }
    onOpen(featureName);
  }

  async function continueCreate() {
    if (!createCheck || createCheck.blocked) return;
    if (
      existingBranch === "recreate" &&
      !(await confirm({
        title: t("features.preflightRecreateTitle"),
        message: t("features.preflightRecreateConfirm"),
        confirmLabel: t("features.preflightContinue"),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await completeCreate(createCheck.name, existingBranch);
    } catch (err) {
      // State can change after the initial check. Refresh the conflict view
      // instead of leaving the user with a generic retry error.
      try {
        const refreshed = await api.CheckFeatureCreation(createCheck.name, "");
        if (refreshed.hasConflicts || refreshed.blocked) {
          setCreateCheck(refreshed);
        } else {
          notify(errMessage(err), "error");
        }
      } catch {
        notify(errMessage(err), "error");
      }
    } finally {
      setBusy(false);
    }
  }

  const conflictRepos = createCheck?.repositories?.filter((repo) => repo.conflict || repo.blockedReason) ?? [];
  const canReuse = conflictRepos.every((repo) => !repo.conflict || repo.canReuse);
  const canRecreate = conflictRepos.every((repo) => !repo.conflict || repo.canRecreate);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("features.title")}</CardTitle>
            <CardDescription>
              {t("features.count", { count: features.length })}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" /> {t("common.refresh")}
          </Button>
        </CardHeader>
        <CardContent>
          {features.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("features.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("features.colFeature")}</TableHead>
                  <TableHead>{t("features.colBranch")}</TableHead>
                  <TableHead>{t("features.colRepos")}</TableHead>
                  <TableHead>{t("features.colAgent")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {features.map((f) => {
                  const terminals = terminalTabs[f.name] ?? [];
                  const terminalExpanded = expanded.has(f.name);
                  const syncBusy = syncing[f.name] ?? false;
                  const syncError = syncErrors[f.name];
                  const activeTerminalSession =
                    terminals.find((tab) => tab.id === activeTerminal[f.name]) ??
                    terminals[terminals.length - 1];
                  return (
                    <Fragment key={f.name}>
                      <TableRow
                        className="cursor-pointer"
                        onClick={() => onOpen(f.name)}
                      >
                        <TableCell className="font-medium">{f.name}</TableCell>
                        <TableCell>{f.branch}</TableCell>
                        <TableCell>{f.repoCount}</TableCell>
                        <TableCell>
                          {f.agentReady ? (
                            <Badge variant="success">{t("features.agentReady")}</Badge>
                          ) : (
                            <Badge variant="outline">{t("features.agentNone")}</Badge>
                          )}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              title={t("features.openFolder")}
                              onClick={(e) => {
                                e.stopPropagation();
                                openFolder(f.name);
                              }}
                            >
                              <FolderOpen className="size-4" />
                            </Button>
                            <div className="relative">
                              <Button
                                variant={terminalExpanded ? "secondary" : "ghost"}
                                size="icon"
                                title={t("features.openTerminal")}
                                disabled={busy && terminals.length === 0}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  togglePanel(f.name);
                                }}
                              >
                                <Terminal className="size-4" />
                              </Button>
                              {terminals.length > 0 && (
                                <span className="pointer-events-none absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium leading-none text-primary-foreground">
                                  {terminals.length}
                                </span>
                              )}
                            </div>
                            <Button
                              variant="ghost"
                              size="icon"
                              title={t("features.openVSCode")}
                              onClick={(e) => {
                                e.stopPropagation();
                                openVSCode(f.name);
                              }}
                            >
                              <Code2 className="size-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              title={t("feature.delete")}
                              disabled={busy}
                              onClick={(e) => {
                                e.stopPropagation();
                                deleteFeature(f.name);
                              }}
                            >
                              <Trash2 className="size-4 text-destructive" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                      {terminalExpanded && terminals.length > 0 && (
                        <TableRow>
                          <TableCell colSpan={5} className="bg-muted/20 p-0" onClick={(e) => e.stopPropagation()}>
                            <div className="border-t">
                              <div className="flex items-center gap-1 overflow-x-auto border-b bg-background px-2 py-1.5">
                                {terminals.map((term) => (
                                  <div
                                    key={term.id}
                                    className={
                                      "flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-sm transition-colors " +
                                      (activeTerminalSession?.id === term.id
                                        ? "bg-secondary font-medium text-secondary-foreground"
                                        : "text-muted-foreground hover:text-foreground")
                                    }
                                    title={term.path}
                                  >
                                    <button
                                      type="button"
                                      className="flex min-w-0 items-center gap-1"
                                      onClick={() =>
                                        setActiveTerminal((prev) => ({ ...prev, [f.name]: term.id }))
                                      }
                                    >
                                      <Terminal className="size-3.5 shrink-0" />
                                      <span className="max-w-32 truncate">{term.title}</span>
                                    </button>
                                    <button
                                      type="button"
                                      className="rounded p-0.5 opacity-70 hover:bg-accent hover:opacity-100"
                                      onClick={() => void closeTerminal(f.name, term.id)}
                                      title={t("common.close")}
                                    >
                                      <X className="size-3" />
                                    </button>
                                  </div>
                                ))}
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="size-7 shrink-0"
                                  title={t("features.openTerminal")}
                                  disabled={busy}
                                  onClick={() => void openNewTerminal(f.name)}
                                >
                                  <Plus className="size-4" />
                                </Button>
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="shrink-0"
                                  disabled={busy || syncBusy}
                                  onClick={() => void syncFeature(f.name)}
                                  title={t("features.syncAgentChanges")}
                                >
                                  {syncBusy ? (
                                    <Loader2 className="size-4 animate-spin" />
                                  ) : (
                                    <Upload className="size-4" />
                                  )}
                                  {syncBusy ? t("features.syncing") : t("features.syncAgentChanges")}
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="ml-auto shrink-0"
                                  onClick={() => collapsePanel(f.name)}
                                  title={t("features.collapseTerminal")}
                                >
                                  <ChevronUp className="size-4" />
                                </Button>
                              </div>
                              {syncError && (
                                <div className="flex items-start justify-between gap-3 border-b border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950">
                                  <div className="flex min-w-0 gap-2">
                                    <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                                    <span className="whitespace-pre-wrap break-words">{syncError}</span>
                                  </div>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="shrink-0 bg-white"
                                    onClick={() => onOpen(f.name)}
                                  >
                                    {t("features.openDetail")}
                                  </Button>
                                </div>
                              )}
                              <div
                                className="relative w-full overflow-hidden"
                                style={{ height: heights[f.name] ?? DEFAULT_TERMINAL_HEIGHT }}
                              >
                                {activeTerminalSession && (
                                  <div className="absolute inset-0 flex flex-col">
                                    <TerminalPanel
                                      key={activeTerminalSession.id}
                                      id={activeTerminalSession.id}
                                      path={activeTerminalSession.path}
                                      className="flex h-full flex-col"
                                    />
                                  </div>
                                )}
                              </div>
                              <div
                                className="flex h-2 cursor-row-resize items-center justify-center border-t bg-muted/40 hover:bg-muted"
                                onPointerDown={(e) => startResize(f.name, e)}
                                title={t("features.resizeTerminal")}
                              >
                                <div className="h-0.5 w-10 rounded bg-muted-foreground/40" />
                              </div>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("features.createTitle")}</CardTitle>
          <CardDescription>{t("features.createDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={create} className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="fn">{t("features.nameLabel")}</Label>
              <Input
                id="fn"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="coupon-v2"
              />
            </div>
            <div className="sm:col-span-2">
              <Button type="submit" disabled={busy}>
                <Plus className="size-4" /> {t("features.createButton")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {createCheck && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => !busy && setCreateCheck(null)}
        >
          <div
            className="w-full max-w-2xl rounded-lg border bg-card p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">{t("features.preflightTitle")}</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("features.preflightDesc", { branch: createCheck.branch })}
            </p>

            <div className="mt-4 max-h-64 divide-y overflow-auto rounded-md border">
              {conflictRepos.map((repo) => (
                <div key={repo.name} className="space-y-1 p-3 text-sm">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-medium">{repo.name}</span>
                    {repo.blockedReason ? (
                      <Badge variant="destructive">{t("features.preflightBlocked")}</Badge>
                    ) : (
                      <Badge variant="outline">{t("features.preflightExisting")}</Badge>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {[
                      repo.localBranch && t("features.preflightLocal"),
                      repo.remoteBranch && t("features.preflightRemote"),
                      repo.checkedOutAt &&
                        t("features.preflightCheckedOut", { path: repo.checkedOutAt }),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                  {repo.blockedReason && (
                    <p className="text-xs text-destructive">{repo.blockedReason}</p>
                  )}
                </div>
              ))}
            </div>

            {!createCheck.blocked && (
              <div className="mt-4 space-y-2">
                <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
                  <input
                    type="radio"
                    name="branchPolicy"
                    value="reuse"
                    checked={existingBranch === "reuse"}
                    disabled={!canReuse}
                    onChange={() => setExistingBranch("reuse")}
                  />
                  <span>
                    <strong>{t("features.existingBranchReuse")}</strong>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {t("features.existingBranchHint.reuse")}
                    </span>
                  </span>
                </label>
                <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
                  <input
                    type="radio"
                    name="branchPolicy"
                    value="recreate"
                    checked={existingBranch === "recreate"}
                    disabled={!canRecreate}
                    onChange={() => setExistingBranch("recreate")}
                  />
                  <span>
                    <strong>{t("features.existingBranchRecreate")}</strong>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {t("features.existingBranchHint.recreate")}
                    </span>
                  </span>
                </label>
              </div>
            )}

            <div className="mt-5 flex justify-end gap-2">
              <Button variant="outline" disabled={busy} onClick={() => setCreateCheck(null)}>
                {createCheck.blocked ? t("common.close") : t("common.cancel")}
              </Button>
              {!createCheck.blocked && (
                <Button
                  variant={existingBranch === "recreate" ? "destructive" : "default"}
                  disabled={busy || (existingBranch === "reuse" ? !canReuse : !canRecreate)}
                  onClick={continueCreate}
                >
                  {t("features.preflightContinue")}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
