import { useCallback, useEffect, useState } from "react";
import {
  Download,
  FileX2,
  LayoutTemplate,
  Plus,
  Save,
  Trash2,
  Upload,
  Wand2,
} from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { api, errMessage } from "@/lib/api";
import type { Config, MaskRule, SecurityTemplate } from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  config: Config | null;
}

export function AgentSecurityPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [busy, setBusy] = useState(false);
  // Bumped after import to remount the cards so they reload from disk.
  const [reloadToken, setReloadToken] = useState(0);

  if (!config) {
    return (
      <div className="max-w-2xl space-y-6">
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

  return (
    <div className="max-w-2xl space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("agentsec.transferTitle")}</CardTitle>
          <CardDescription>{t("agentsec.transferDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button onClick={doExport} disabled={busy}>
            <Download className="size-4" /> {t("agentsec.export")}
          </Button>
          <Button variant="outline" onClick={doImport} disabled={busy}>
            <Upload className="size-4" /> {t("agentsec.import")}
          </Button>
        </CardContent>
      </Card>
      <TemplatesCard onApplied={() => setReloadToken((n) => n + 1)} />
      <IgnoreCard key={`ig-${reloadToken}`} />
      <MaskCard key={`mk-${reloadToken}`} />
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
