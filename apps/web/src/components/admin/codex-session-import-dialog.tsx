"use client";

import { useState } from "react";
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
import { useImportCodexSession } from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import type { CodexSessionImportResult, Id } from "@/lib/sdk-types";

/**
 * Paste a Codex/ChatGPT desktop session blob (session JSON, a raw access token,
 * or an NDJSON batch) to onboard upstream codex_cli accounts. Decodes the
 * embedded JWT server-side; the browser never sees minted tokens.
 */
export function CodexSessionImportDialog({
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
  const importMut = useImportCodexSession();

  const [providerId, setProviderId] = useState<string>(defaultProviderId);
  const [content, setContent] = useState<string>("");
  const [name, setName] = useState<string>("");
  const [updateExisting, setUpdateExisting] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<CodexSessionImportResult | null>(null);

  function reset() {
    setContent("");
    setName("");
    setError(null);
    setResult(null);
  }

  async function submit() {
    setError(null);
    if (!providerId) {
      setError(t("codexImport.providerRequired"));
      return;
    }
    if (!content.trim()) {
      setError(t("codexImport.contentRequired"));
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
      const summary = t("codexImport.doneSummary", {
        created: data.created,
        updated: data.updated,
        skipped: data.skipped,
        failed: data.failed,
      });
      toast({
        title: t("codexImport.done"),
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
          <DialogTitle>{t("codexImport.title")}</DialogTitle>
          <DialogDescription>{t("codexImport.subtitle")}</DialogDescription>
        </DialogHeader>

        <div className="mt-2 space-y-4">
          <div>
            <Label htmlFor="codex-import-provider">{t("codexImport.provider")}</Label>
            <Select value={providerId} onValueChange={setProviderId} disabled={importMut.isPending}>
              <SelectTrigger id="codex-import-provider">
                <SelectValue placeholder={t("codexImport.providerPlaceholder")} />
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
            <Label htmlFor="codex-import-content">{t("codexImport.content")}</Label>
            <Textarea
              id="codex-import-content"
              rows={8}
              spellCheck={false}
              className="font-mono text-xs"
              placeholder={t("codexImport.contentPlaceholder")}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              disabled={importMut.isPending}
            />
            <p className="mt-1 text-xs text-srapi-text-tertiary">{t("codexImport.contentHint")}</p>
          </div>

          <div>
            <Label htmlFor="codex-import-name">{t("codexImport.name")}</Label>
            <Input
              id="codex-import-name"
              placeholder={t("codexImport.namePlaceholder")}
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={importMut.isPending}
            />
          </div>

          <div className="flex items-center justify-between rounded-md border border-srapi-border px-3 py-2">
            <div>
              <Label htmlFor="codex-import-update" className="cursor-pointer">
                {t("codexImport.updateExisting")}
              </Label>
              <p className="text-xs text-srapi-text-tertiary">{t("codexImport.updateExistingHint")}</p>
            </div>
            <Switch
              id="codex-import-update"
              checked={updateExisting}
              onCheckedChange={setUpdateExisting}
              disabled={importMut.isPending}
            />
          </div>

          {result ? <CodexImportResultPanel result={result} /> : null}

          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
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
            {t("codexImport.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CodexImportResultPanel({ result }: { result: CodexSessionImportResult }) {
  const { t } = useLanguage();
  return (
    <div className="space-y-3 rounded-md border border-srapi-border bg-srapi-card-muted p-3">
      <div className="grid grid-cols-4 gap-2 text-center">
        <CodexStat label={t("codexImport.created")} value={result.created} tone="success" />
        <CodexStat label={t("codexImport.updated")} value={result.updated} />
        <CodexStat label={t("codexImport.skipped")} value={result.skipped} />
        <CodexStat label={t("codexImport.failed")} value={result.failed} tone="error" />
      </div>
      {result.errors.length > 0 ? (
        <ul className="space-y-1 text-xs text-srapi-error">
          {result.errors.map((msg, idx) => (
            <li key={`err-${idx}`}>
              #{msg.index}
              {msg.name ? ` ${msg.name}` : ""}: {msg.message}
            </li>
          ))}
        </ul>
      ) : null}
      {result.warnings.length > 0 ? (
        <ul className="space-y-1 text-xs text-srapi-text-tertiary">
          {result.warnings.map((msg, idx) => (
            <li key={`warn-${idx}`}>
              #{msg.index}
              {msg.name ? ` ${msg.name}` : ""}: {msg.message}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function CodexStat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "success" | "error";
}) {
  const toneClass =
    tone === "success"
      ? "text-srapi-success"
      : tone === "error"
        ? "text-srapi-error"
        : "text-srapi-text-primary";
  return (
    <div>
      <div className={`text-lg font-semibold ${toneClass}`}>{value}</div>
      <div className="text-xs text-srapi-text-tertiary">{label}</div>
    </div>
  );
}
