import { useCallback, useEffect, useState } from "react";
import {
  Download,
  Eye,
  FileX2,
  LayoutTemplate,
  Plus,
  Save,
  Search,
  Trash2,
  Upload,
  Wand2,
  X,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { api, errMessage } from "@/lib/api";
import type {
  Config,
  FileViewSide,
  MaskRule,
  PreviewEntry,
  PreviewResult,
  PreviewStatus,
  Repository,
  SecurityPreviewFile,
  SecurityTemplate,
} from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  config: Config | null;
}

type SecTab = "templates" | "ignore" | "mask" | "preview";

export function AgentSecurityPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [busy, setBusy] = useState(false);
  // Bumped after import to remount the cards so they reload from disk.
  const [reloadToken, setReloadToken] = useState(0);
  const [tab, setTab] = useState<SecTab>("templates");

  if (!config) {
    return (
      <div className="max-w-4xl space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t("nav.agentSecurity")}</CardTitle>
            <CardDescription>{t("agentsec.noWorkspace")}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  async function doExport() {
    try {
      setBusy(true);
      const p = await api.ExportAgentSecurity();
      if (p) notify(t("agentsec.exported", { path: p }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function doImport() {
    if (!(await confirm({ message: t("agentsec.importConfirm"), danger: true })))
      return;
    try {
      setBusy(true);
      const p = await api.ImportAgentSecurity();
      if (p) {
        setReloadToken((n) => n + 1);
        notify(t("agentsec.imported"), "success");
      }
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  const tabs: { id: SecTab; label: string }[] = [
    { id: "templates", label: t("agentsec.tabTemplates") },
    { id: "ignore", label: t("agentsec.tabIgnore") },
    { id: "mask", label: t("agentsec.tabMask") },
    { id: "preview", label: t("agentsec.tabPreview") },
  ];

  return (
    <div className="max-w-4xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {t("agentsec.transferDesc")}
        </p>
        <div className="flex shrink-0 gap-2">
          <Button onClick={doExport} disabled={busy}>
            <Download className="size-4" /> {t("agentsec.export")}
          </Button>
          <Button variant="outline" onClick={doImport} disabled={busy}>
            <Upload className="size-4" /> {t("agentsec.import")}
          </Button>
        </div>
      </div>

      <div className="inline-flex rounded-lg border bg-card p-1">
        {tabs.map((tb) => (
          <button
            key={tb.id}
            onClick={() => setTab(tb.id)}
            className={cn(
              "rounded-md px-3 py-1.5 text-sm transition-colors",
              tab === tb.id
                ? "bg-secondary font-medium text-secondary-foreground"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            {tb.label}
          </button>
        ))}
      </div>

      {tab === "templates" && (
        <TemplatesCard onApplied={() => setReloadToken((n) => n + 1)} />
      )}
      {tab === "ignore" && <IgnoreCard key={`ig-${reloadToken}`} />}
      {tab === "mask" && <MaskCard key={`mk-${reloadToken}`} />}
      {tab === "preview" && <PreviewTab />}
    </div>
  );
}

function TemplatesCard({ onApplied }: { onApplied: () => void }) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [templates, setTemplates] = useState<SecurityTemplate[]>([]);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setTemplates(await api.ListSecurityTemplates());
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  function toggle(key: string) {
    setSelected((s) => ({ ...s, [key]: !s[key] }));
  }

  async function apply(replace: boolean) {
    const keys = templates.map((t) => t.key).filter((k) => selected[k]);
    if (keys.length === 0) {
      notify(t("agentsec.tplNoSelection"), "error");
      return;
    }
    if (
      replace &&
      !(await confirm({ message: t("agentsec.tplReplaceConfirm"), danger: true }))
    )
      return;
    try {
      setBusy(true);
      await api.ApplySecurityTemplates(keys, replace);
      setSelected({});
      onApplied();
      notify(t("agentsec.tplApplied"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LayoutTemplate className="size-5" /> {t("agentsec.tplTitle")}
        </CardTitle>
        <CardDescription>{t("agentsec.tplDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {templates.map((tpl) => (
            <button
              key={tpl.key}
              onClick={() => toggle(tpl.key)}
              className={cn(
                "rounded-md border p-3 text-left transition-colors",
                selected[tpl.key]
                  ? "border-primary bg-primary/5"
                  : "bg-card hover:bg-muted/50"
              )}
            >
              <div className="flex items-center justify-between">
                <span className="font-medium">{tpl.label}</span>
                <span className="text-xs text-muted-foreground">
                  {t("agentsec.tplCounts", {
                    ignore: tpl.ignoreCount,
                    mask: tpl.maskCount,
                  })}
                </span>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                {tpl.description}
              </p>
            </button>
          ))}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => apply(false)} disabled={busy}>
            <Plus className="size-4" /> {t("agentsec.tplApply")}
          </Button>
          <Button
            variant="outline"
            onClick={() => apply(true)}
            disabled={busy}
          >
            {t("agentsec.tplReplace")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function IgnoreCard() {
  const { t } = useI18n();
  const { notify } = useToast();
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setContent(await api.GetAgentIgnore());
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  async function save() {
    try {
      setBusy(true);
      await api.SaveAgentIgnore(content);
      notify(t("toast.agentIgnoreSaved"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileX2 className="size-5" /> {t("agentsec.ignoreTitle")}
        </CardTitle>
        <CardDescription>{t("agentsec.ignoreDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder={t("agentsec.ignorePlaceholder")}
          spellCheck={false}
        />
        <Button onClick={save} disabled={busy}>
          <Save className="size-4" /> {t("agentsec.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

type MaskTab = "structured" | "json";

function MaskCard() {
  const { t } = useI18n();
  const { notify } = useToast();
  const [tab, setTab] = useState<MaskTab>("structured");
  const [rules, setRules] = useState<MaskRule[]>([]);
  const [jsonText, setJsonText] = useState("[]");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const m = await api.GetMaskFile();
      const rs = m.rules ?? [];
      setRules(rs);
      setJsonText(JSON.stringify(rs, null, 2));
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  // Parse the JSON editor into rules; returns null and notifies on failure.
  function parseJson(): MaskRule[] | null {
    try {
      const parsed = JSON.parse(jsonText);
      if (!Array.isArray(parsed)) throw new Error("not an array");
      return parsed as MaskRule[];
    } catch {
      notify(t("agentsec.jsonInvalid"), "error");
      return null;
    }
  }

  function switchTab(next: MaskTab) {
    if (next === tab) return;
    if (next === "json") {
      // structured -> json
      setJsonText(JSON.stringify(rules, null, 2));
      setTab("json");
    } else {
      // json -> structured (block on invalid JSON)
      const parsed = parseJson();
      if (!parsed) return;
      setRules(parsed);
      setTab("structured");
    }
  }

  function updateRule(i: number, patch: Partial<MaskRule>) {
    setRules((rs) => rs.map((r, n) => (n === i ? { ...r, ...patch } : r)));
  }

  function addRule() {
    setRules((rs) => [
      ...rs,
      { name: "", type: "plain", pattern: "", replacement: "" },
    ]);
  }

  function removeRule(i: number) {
    setRules((rs) => rs.filter((_, n) => n !== i));
  }

  async function save() {
    let next = rules;
    if (tab === "json") {
      const parsed = parseJson();
      if (!parsed) return;
      next = parsed;
      setRules(parsed);
    }
    try {
      setBusy(true);
      await api.SaveMaskFile({ rules: next });
      setJsonText(JSON.stringify(next, null, 2));
      notify(t("toast.maskSaved"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  const tabs: { id: MaskTab; label: string }[] = [
    { id: "structured", label: t("agentsec.tabStructured") },
    { id: "json", label: t("agentsec.tabJson") },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Wand2 className="size-5" /> {t("agentsec.maskTitle")}
        </CardTitle>
        <CardDescription>{t("agentsec.maskDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="inline-flex rounded-lg border bg-card p-1">
          {tabs.map((tb) => (
            <button
              key={tb.id}
              onClick={() => switchTab(tb.id)}
              className={cn(
                "rounded-md px-3 py-1.5 text-sm transition-colors",
                tab === tb.id
                  ? "bg-secondary font-medium text-secondary-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              {tb.label}
            </button>
          ))}
        </div>

        {tab === "structured" ? (
          <div className="space-y-3">
            {rules.length === 0 && (
              <p className="text-sm text-muted-foreground">
                {t("agentsec.noRules")}
              </p>
            )}
            {rules.map((r, i) => (
              <div
                key={i}
                className="space-y-2 rounded-md border bg-muted/30 p-3"
              >
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label>{t("agentsec.ruleName")}</Label>
                    <Input
                      value={r.name}
                      onChange={(e) => updateRule(i, { name: e.target.value })}
                      placeholder="AWS Access Key"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>{t("agentsec.ruleType")}</Label>
                    <select
                      value={r.type}
                      onChange={(e) => updateRule(i, { type: e.target.value })}
                      className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    >
                      <option value="plain">{t("agentsec.typePlain")}</option>
                      <option value="regex">{t("agentsec.typeRegex")}</option>
                      <option value="keypath">
                        {t("agentsec.typeKeyPath")}
                      </option>
                    </select>
                  </div>
                </div>
                <div className="space-y-1">
                  <Label>{t("agentsec.rulePattern")}</Label>
                  <Input
                    value={r.pattern}
                    onChange={(e) => updateRule(i, { pattern: e.target.value })}
                    placeholder={
                      r.type === "keypath" ? "main.sub" : "AKIA[0-9A-Z]{16}"
                    }
                    className="font-mono"
                  />
                  {r.type === "keypath" && (
                    <p className="text-xs text-muted-foreground">
                      {t("agentsec.keyPathHint")}
                    </p>
                  )}
                </div>
                <div className="space-y-1">
                  <Label>{t("agentsec.ruleReplacement")}</Label>
                  <Input
                    value={r.replacement}
                    onChange={(e) =>
                      updateRule(i, { replacement: e.target.value })
                    }
                    placeholder="__MASKED__"
                    className="font-mono"
                  />
                </div>
                <div className="flex justify-end">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeRule(i)}
                  >
                    <Trash2 className="size-4" /> {t("agentsec.removeRule")}
                  </Button>
                </div>
              </div>
            ))}
            <Button variant="outline" onClick={addRule}>
              <Plus className="size-4" /> {t("agentsec.addRule")}
            </Button>
          </div>
        ) : (
          <Textarea
            value={jsonText}
            onChange={(e) => setJsonText(e.target.value)}
            className="min-h-[260px]"
            spellCheck={false}
          />
        )}

        <Button onClick={save} disabled={busy}>
          <Save className="size-4" /> {t("agentsec.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

type StatusFilter = "all" | PreviewStatus;

const STATUS_BADGE: Record<
  PreviewStatus,
  { variant: "secondary" | "warning" | "outline"; key: string }
> = {
  ignored: { variant: "secondary", key: "agentsec.statusIgnored" },
  masked: { variant: "warning", key: "agentsec.statusMasked" },
  copied: { variant: "outline", key: "agentsec.statusCopied" },
};

// FileDiffPanel mirrors FeatureDetailPage's FileViewSidePanel: a titled,
// scrollable content pane for one side of the before/after mask comparison.
function FileDiffPanel({ title, side }: { title: string; side: FileViewSide }) {
  return (
    <div className="min-w-0 overflow-hidden rounded-md border">
      <div className="border-b bg-muted/40 px-3 py-2 font-medium">{title}</div>
      {!side.exists ? (
        <div className="p-4 text-sm text-muted-foreground">—</div>
      ) : side.error ? (
        <div className="p-4 text-sm text-destructive">{side.error}</div>
      ) : (
        <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-words bg-slate-950 p-3 font-mono text-xs text-slate-100">
          {side.content ?? ""}
        </pre>
      )}
    </div>
  );
}

function PreviewTab() {
  const { t } = useI18n();
  const { notify } = useToast();
  const [repos, setRepos] = useState<Repository[]>([]);
  const [repo, setRepo] = useState("");
  const [result, setResult] = useState<PreviewResult | null>(null);
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [busy, setBusy] = useState(false);
  // On-demand before/after viewer for a masked file.
  const [view, setView] = useState<{
    path: string;
    loading: boolean;
    data?: SecurityPreviewFile;
    error?: string;
  } | null>(null);

  useEffect(() => {
    api
      .ListRepos()
      .then((rs) => {
        setRepos(rs);
        setRepo((prev) => prev || (rs[0]?.Name ?? ""));
      })
      .catch((e) => notify(errMessage(e), "error"));
  }, [notify]);

  async function scan() {
    if (!repo) return;
    try {
      setBusy(true);
      setResult(await api.ScanSecurityPreview(repo));
      setFilter("all");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  function openDiff(path: string) {
    setView({ path, loading: true });
    api
      .ScanSecurityPreviewFile(repo, path)
      .then((data) => setView({ path, loading: false, data }))
      .catch((e) => setView({ path, loading: false, error: errMessage(e) }));
  }

  const entries = result?.entries ?? [];
  const filtered =
    filter === "all" ? entries : entries.filter((e) => e.status === filter);

  const chips: { id: StatusFilter; label: string; count: number }[] = [
    { id: "all", label: t("agentsec.filterAll"), count: result?.total ?? 0 },
    {
      id: "ignored",
      label: t("agentsec.statIgnored"),
      count: result?.ignored ?? 0,
    },
    { id: "masked", label: t("agentsec.statMasked"), count: result?.masked ?? 0 },
    { id: "copied", label: t("agentsec.statCopied"), count: result?.copied ?? 0 },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Search className="size-5" /> {t("agentsec.previewTitle")}
        </CardTitle>
        <CardDescription>{t("agentsec.previewDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-muted-foreground">
          {t("agentsec.previewSavedHint")}
        </p>

        {repos.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("agentsec.previewNoRepos")}
          </p>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <div className="space-y-1">
              <Label>{t("agentsec.previewRepo")}</Label>
              <select
                value={repo}
                onChange={(e) => setRepo(e.target.value)}
                className="flex h-9 min-w-48 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                {repos.map((r) => (
                  <option key={r.Name} value={r.Name}>
                    {r.Name}
                  </option>
                ))}
              </select>
            </div>
            <Button onClick={scan} disabled={busy || !repo}>
              <Search className="size-4" /> {t("agentsec.previewScan")}
            </Button>
          </div>
        )}

        {result && (
          <div className="flex flex-wrap gap-2">
            {chips.map((c) => (
              <button
                key={c.id}
                onClick={() => setFilter(c.id)}
                className={cn(
                  "rounded-md border px-3 py-1.5 text-sm transition-colors",
                  filter === c.id
                    ? "border-primary bg-primary/5 font-medium"
                    : "bg-card hover:bg-muted/50"
                )}
              >
                {c.label}{" "}
                <span className="text-muted-foreground">{c.count}</span>
              </button>
            ))}
          </div>
        )}

        {!result ? (
          <p className="text-sm text-muted-foreground">
            {t("agentsec.previewEmpty")}
          </p>
        ) : filtered.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("agentsec.previewNoMatch")}
          </p>
        ) : (
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-24">
                    {t("agentsec.colStatus")}
                  </TableHead>
                  <TableHead>{t("agentsec.colPath")}</TableHead>
                  <TableHead>{t("agentsec.colDetail")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((e) => (
                  <PreviewRow
                    key={e.path}
                    entry={e}
                    onView={() => openDiff(e.path)}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>

      {view && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => setView(null)}
        >
          <div
            className="flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-lg border bg-card shadow-xl"
            onClick={(ev) => ev.stopPropagation()}
          >
            <div className="flex items-start justify-between gap-4 border-b p-4">
              <h2 className="min-w-0 truncate text-lg font-semibold">
                {t("agentsec.diffTitle", { path: view.path })}
              </h2>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setView(null)}
              >
                <X className="size-4" />
              </Button>
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-4">
              {view.loading ? (
                <p className="text-sm text-muted-foreground">…</p>
              ) : view.error ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
                  {view.error}
                </div>
              ) : view.data ? (
                <div className="grid gap-4 lg:grid-cols-2">
                  <FileDiffPanel
                    title={t("agentsec.diffBefore")}
                    side={view.data.before}
                  />
                  <FileDiffPanel
                    title={t("agentsec.diffAfter")}
                    side={view.data.after}
                  />
                </div>
              ) : null}
            </div>
          </div>
        </div>
      )}
    </Card>
  );
}

function PreviewRow({
  entry,
  onView,
}: {
  entry: PreviewEntry;
  onView: () => void;
}) {
  const { t } = useI18n();
  const badge = STATUS_BADGE[entry.status];
  const matches = entry.maskMatches ?? [];
  return (
    <TableRow>
      <TableCell className="align-top">
        <Badge variant={badge.variant}>{t(badge.key)}</Badge>
      </TableCell>
      <TableCell className="align-top font-mono text-xs">{entry.path}</TableCell>
      <TableCell className="align-top">
        {entry.status === "ignored" && (
          <span className="font-mono text-xs text-muted-foreground">
            {entry.ignorePattern}
          </span>
        )}
        {entry.status === "copied" && entry.binary && (
          <span className="text-xs text-muted-foreground">
            {t("agentsec.binary")}
          </span>
        )}
        {entry.status === "masked" && (
          <div className="flex flex-wrap items-center gap-1.5">
            {matches.map((m, i) => (
              <Badge key={i} variant="outline" className="font-normal">
                {(m.name || m.type) + " ×" + m.count}
              </Badge>
            ))}
            <Button
              variant="ghost"
              size="sm"
              className="h-7"
              onClick={onView}
              title={t("agentsec.viewDiff")}
            >
              <Eye className="size-3.5" /> {t("agentsec.viewDiff")}
            </Button>
          </div>
        )}
      </TableCell>
    </TableRow>
  );
}
