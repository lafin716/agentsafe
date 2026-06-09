import { useEffect, useState } from "react";
import { Languages, Save, Stethoscope } from "lucide-react";
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
import { useI18n } from "@/i18n/I18nProvider";
import { LOCALES, type Locale } from "@/i18n/translations";
import { api, errMessage } from "@/lib/api";
import type { Config, GitDiag } from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  config: Config | null;
  onChanged: () => void | Promise<void>;
}

export function SettingsPage({ config, onChanged }: Props) {
  const { locale, setLocale, t } = useI18n();
  const { notify } = useToast();

  function choose(l: Locale) {
    if (l === locale) return;
    setLocale(l);
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
  const [ghUrl, setGhUrl] = useState("");
  const [ghToken, setGhToken] = useState("");
  const [ghTarget, setGhTarget] = useState("");

  useEffect(() => {
    setBaseBranch(config.Git?.DefaultBaseBranch ?? "");
    setBranchPrefix(config.Git?.BranchPrefix ?? "");
    setGlUrl(config.GitLab?.BaseURL ?? "");
    setGlToken(config.GitLab?.TokenEnv ?? "");
    setGlTarget(config.GitLab?.TargetBranch ?? "");
    setGhUrl(config.GitHub?.BaseURL ?? "");
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
          BaseURL: ghUrl.trim(),
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
          <Field label={t("settings.baseUrl")} hint={urlHint(ghUrl)}>
            <Input value={ghUrl} onChange={(e) => setGhUrl(e.target.value)} placeholder="https://github.com" />
          </Field>
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
