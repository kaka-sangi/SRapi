"use client";

import { useCallback, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { cn } from "@/lib/cn";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { FileDropZone } from "@/components/ui/file-drop-zone";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { useImportSession } from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import type { SessionImportResult, Id } from "@/lib/sdk-types";

/**
 * Paste a session blob (session JSON, a raw access token, or an NDJSON batch)
 * to onboard upstream accounts. Decodes the embedded JWT server-side; the
 * browser never sees minted tokens.
 */
function SessionImportDialog({
  open,
  onOpenChange,
  providerOptions,
  defaultProviderId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerOptions: { value: string; label: string }[];
  defaultProviderId: string;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const importMut = useImportSession();

  const [providerId, setProviderId] = useState<string>(defaultProviderId);
  const [content, setContent] = useState<string>("");
  const [name, setName] = useState<string>("");
  const [updateExisting, setUpdateExisting] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SessionImportResult | null>(null);
  const [fileNames, setFileNames] = useState<string[]>([]);

  // Each drop replaces the staged content. Appending (the original
  // behaviour) silently merged a mis-dropped file with the correct one
  // and shipped the bogus session through to import — a real data
  // corruption path observed in ROOTCAUSE-SWEEP. Multi-file imports are
  // still supported in a single drag (the FileDropZone passes the
  // whole File[] in one callback); explicit accumulation can be added
  // back with a UI affordance once it is actually needed.
  const handleFiles = useCallback(async (files: File[]) => {
    const texts = await Promise.all(files.map((f) => f.text()));
    setContent(texts.join("\n"));
    setFileNames(files.map((f) => f.name));
  }, []);

  function reset() {
    setContent("");
    setName("");
    setError(null);
    setResult(null);
    setFileNames([]);
  }

  async function submit() {
    setError(null);
    if (!providerId) {
      setError(t("sessionImport.providerRequired"));
      return;
    }
    if (!content.trim()) {
      setError(t("sessionImport.contentRequired"));
      return;
    }
    try {
      const data = await importMut.mutateAsync({
        provider_id: providerId as Id,
        content,
        name: name.trim() ? name.trim() : undefined,
        update_existing: updateExisting,
      });
      setResult(data);
      const summary = t("sessionImport.doneSummary", {
        created: data.created,
        updated: data.updated,
        skipped: data.skipped,
        failed: data.failed,
      });
      toast({
        title: t("sessionImport.done"),
        description: summary,
        tone: data.failed > 0 ? "error" : "success",
      });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("sessionImport.title")}</DialogTitle>
          <DialogDescription>{t("sessionImport.subtitle")}</DialogDescription>
        </DialogHeader>

        <div className="mt-2 space-y-4">
          <div>
            <Label htmlFor="session-import-provider">{t("sessionImport.provider")}</Label>
            <Select value={providerId} onValueChange={setProviderId} disabled={importMut.isPending}>
              <SelectTrigger id="session-import-provider">
                <SelectValue placeholder={t("sessionImport.providerPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {providerOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div>
            <Label htmlFor="session-import-content">{t("sessionImport.content")}</Label>
            <FileDropZone
              accept=".json,.txt,.ndjson"
              multiple
              disabled={importMut.isPending}
              hint={t("sessionImport.dropHint")}
              onFiles={(files) => void handleFiles(files)}
              fileNames={fileNames}
              onClearFiles={() => { setFileNames([]); setContent(""); }}
              className="mb-2"
            />
            <Textarea
              id="session-import-content"
              rows={8}
              spellCheck={false}
              className="font-mono text-xs"
              placeholder={t("sessionImport.contentPlaceholder")}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              disabled={importMut.isPending}
            />
            <p className="mt-1 text-xs text-srapi-text-tertiary">{t("sessionImport.contentHint")}</p>
          </div>

          <div>
            <Label htmlFor="session-import-name">{t("sessionImport.name")}</Label>
            <Input
              id="session-import-name"
              placeholder={t("sessionImport.namePlaceholder")}
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={importMut.isPending}
            />
          </div>

          <div className="flex items-center justify-between rounded-md border border-srapi-border px-3 py-2">
            <div>
              <Label htmlFor="session-import-update" className="cursor-pointer">
                {t("sessionImport.updateExisting")}
              </Label>
              <p className="text-xs text-srapi-text-tertiary">{t("sessionImport.updateExistingHint")}</p>
            </div>
            <Switch
              id="session-import-update"
              checked={updateExisting}
              onCheckedChange={setUpdateExisting}
              disabled={importMut.isPending}
            />
          </div>

          {result ? <SessionImportResultPanel result={result} /> : null}

          {error ? (
            <div role="alert" className="log-row rounded-lg" data-sev="error">
              <p className="px-3 py-2 text-sm text-srapi-error">{error}</p>
            </div>
          ) : null}
        </div>

        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="ghost"
            disabled={importMut.isPending}
            onClick={() => onOpenChange(false)}
          >
            {t("common.close")}
          </Button>
          <Button
            type="button"
            variant="primary"
            loading={importMut.isPending}
            onClick={() => void submit()}
          >
            {t("sessionImport.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function SessionImportResultPanel({ result }: { result: SessionImportResult }) {
  const { t } = useLanguage();
  const total = result.created + result.updated + result.skipped + result.failed;
  return (
    <div className="space-y-3 rounded-xl border border-srapi-border bg-srapi-card-muted p-3.5">
      <div className="grid grid-cols-4 gap-2 text-center">
        <SessionStat
          label={t("sessionImport.created")}
          value={result.created}
          tone="success"
          tier="primary"
          tooltip={{
            rows: [
              { label: t("sessionImport.updated"), value: result.updated },
              { label: t("sessionImport.skipped"), value: result.skipped },
              { label: t("sessionImport.failed"), value: result.failed },
              { label: t("sessionImport.total") ?? "Total", value: total },
            ],
          }}
        />
        <SessionStat label={t("sessionImport.updated")} value={result.updated} />
        <SessionStat label={t("sessionImport.skipped")} value={result.skipped} tier="tertiary" />
        <SessionStat label={t("sessionImport.failed")} value={result.failed} tone="error" />
      </div>
      {result.errors.length > 0 ? (
        <ul className="space-y-1.5">
          {result.errors.map((msg, idx) => (
            <li
              key={`err-${idx}`}
              className="log-row rounded-md text-xs text-srapi-error"
              data-sev="error"
            >
              <span className="flex items-start gap-1.5 px-2 py-1.5">
                <AlertTriangle className="mt-0.5 size-3 shrink-0" />
                <span className="min-w-0 break-words">
                  #{msg.index}
                  {msg.name ? ` ${msg.name}` : ""}: {msg.message}
                </span>
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {result.warnings.length > 0 ? (
        <ul className="space-y-1.5">
          {result.warnings.map((msg, idx) => (
            <li
              key={`warn-${idx}`}
              className="log-row rounded-md text-xs text-srapi-text-tertiary"
              data-sev="warning"
            >
              <span className="block px-2 py-1.5">
                #{msg.index}
                {msg.name ? ` ${msg.name}` : ""}: {msg.message}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function SessionStat({
  label,
  value,
  tone,
  tier = "secondary",
  tooltip,
}: {
  label: string;
  value: number;
  tone?: "success" | "error";
  tier?: "primary" | "secondary" | "tertiary";
  tooltip?: { title?: string; rows?: { label: string; value: React.ReactNode }[] };
}) {
  const toneClass =
    tone === "success"
      ? "metric-strong-good"
      : tone === "error"
        ? value > 0
          ? "metric-strong-bad"
          : "text-srapi-text-tertiary"
        : tier === "primary"
          ? "metric-primary"
          : tier === "tertiary"
            ? "metric-tertiary"
            : "metric-secondary";
  const numberEl = <div className={cn("tabular cursor-help", toneClass)}>{value}</div>;
  return (
    <div className="rounded-xl bg-srapi-card px-2 py-2">
      {tooltip ? (
        <DataTooltip title={tooltip.title ?? label} primary={value} rows={tooltip.rows}>
          {numberEl}
        </DataTooltip>
      ) : (
        numberEl
      )}
      <div className="mt-0.5 text-[11px] font-medium uppercase tracking-[0.08em] text-srapi-text-tertiary">
        {label}
      </div>
    </div>
  );
}
