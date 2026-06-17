import { useState } from "react";
import { Copy, Check } from "lucide-react";
import { useConfigSnapshot, useImportConfigSnapshot } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { writeClipboard } from "@/components/ui/copy-button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { adminErrorMessage } from "@/lib/admin-api";

/** Backup tab: export the full config snapshot as JSON, or import one (dry-run first). */
export function BackupTab() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const snapshot = useConfigSnapshot();
  const importMut = useImportConfigSnapshot();
  const [importText, setImportText] = useState("");
  const [dryRun, setDryRun] = useState(true);
  const [result, setResult] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const snapshotJson = snapshot.data ? JSON.stringify(snapshot.data, null, 2) : "";

  async function copySnapshot() {
    if (!snapshotJson) return;
    const ok = await writeClipboard(snapshotJson);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }

  async function runImport() {
    setResult(null);
    let body;
    try {
      body = JSON.parse(importText || "{}");
    } catch {
      toast({ title: t("feedback.failed"), description: "Invalid JSON", tone: "error" });
      return;
    }
    try {
      const res = await importMut.mutateAsync({ body, dryRun });
      setResult(JSON.stringify(res, null, 2));
      toast({ title: dryRun ? t("adminSettings.dryRun") : t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="font-serif text-lg text-srapi-text-primary">{t("adminSettings.export")}</h3>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => snapshot.refetch()} loading={snapshot.isFetching}>
                {t("adminSettings.fetchSnapshot")}
              </Button>
              {snapshotJson ? (
                <Button variant="outline" size="sm" onClick={copySnapshot}>
                  {copied ? <Check className="size-4 text-srapi-success" /> : <Copy className="size-4" />}
                  {t("common.copy")}
                </Button>
              ) : null}
            </div>
          </div>
          <p className="text-2xs text-srapi-text-tertiary">{t("adminSettings.exportHint")}</p>
          <Textarea
            readOnly
            className="min-h-64 font-mono text-xs"
            value={snapshotJson}
            placeholder={t("adminSettings.fetchSnapshot")}
          />
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-3">
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("adminSettings.import")}</h3>
          <p className="text-2xs text-srapi-text-tertiary">{t("adminSettings.importHint")}</p>
          <Textarea
            className="min-h-48 font-mono text-xs"
            spellCheck={false}
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder='{ "providers": [], "models": [] }'
          />
          <div className="flex items-center justify-between gap-4">
            <label className="flex items-center gap-2 text-sm text-srapi-text-secondary">
              <Switch checked={dryRun} onCheckedChange={setDryRun} />
              {t("adminSettings.dryRun")}
            </label>
            <Button
              variant={dryRun ? "outline" : "primary"}
              size="sm"
              loading={importMut.isPending}
              disabled={!importText.trim()}
              onClick={runImport}
            >
              {dryRun ? t("adminSettings.dryRun") : t("adminSettings.applyImport")}
            </Button>
          </div>
          {result ? (
            <div>
              <Label>{t("adminSettings.importResult")}</Label>
              <Textarea readOnly className="min-h-32 font-mono text-xs" value={result} />
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
