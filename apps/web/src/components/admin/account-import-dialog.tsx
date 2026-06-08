"use client";

import { useCallback, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { FileDropZone } from "@/components/ui/file-drop-zone";
import { useImportAccounts } from "@/hooks/admin-queries";
import { buildImportAccountsBody } from "@/lib/admin-account-form";
import { adminErrorMessage } from "@/lib/admin-api";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import type { ProviderAccountImportResult } from "@/lib/sdk-types";

export function AccountImportDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const importMut = useImportAccounts();
  const [json, setJson] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ProviderAccountImportResult | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);

  const handleFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return;
    const text = await files[0].text();
    setJson(text);
    setFileName(files[0].name);
  }, []);

  function reset() {
    setJson("");
    setError(null);
    setResult(null);
    setFileName(null);
  }

  async function submit() {
    setError(null);
    let body: ReturnType<typeof buildImportAccountsBody>;
    try {
      body = buildImportAccountsBody(json);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Invalid import JSON.");
      return;
    }
    try {
      const res = await importMut.mutateAsync(body);
      setResult(res);
      toast({
        title: t("adminAccounts.importDone", {
          created: res.created_count,
          skipped: res.skipped_count,
        }),
        tone: res.errors.length > 0 ? "default" : "success",
      });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) reset();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("adminAccounts.importTitle")}</DialogTitle>
          <DialogDescription>{t("adminAccounts.importHint")}</DialogDescription>
        </DialogHeader>
        <div className="mt-4 space-y-3">
          <div>
            <Label htmlFor="import-json">{t("adminAccounts.importJson")}</Label>
            <FileDropZone
              accept=".json"
              disabled={importMut.isPending}
              hint={t("adminAccounts.importDropHint")}
              onFiles={(files) => void handleFiles(files)}
              fileNames={fileName ? [fileName] : undefined}
              onClearFiles={() => { setFileName(null); setJson(""); }}
              className="mb-2"
            />
            <Textarea
              id="import-json"
              rows={10}
              spellCheck={false}
              className="font-mono text-xs"
              value={json}
              onChange={(e) => setJson(e.target.value)}
              placeholder={'{ "accounts": [ { "name": "...", "provider_id": "...", "runtime_class": "...", "credential": { } } ] }'}
              disabled={importMut.isPending}
            />
          </div>
          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}
          {result && result.errors.length > 0 ? (
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3">
              <p className="text-2xs font-medium text-srapi-text-secondary">
                {t("adminAccounts.importErrorsTitle")}
              </p>
              <ul className="mt-1 list-disc space-y-0.5 pl-4 text-2xs text-srapi-text-tertiary">
                {result.errors.map((message, idx) => (
                  <li key={idx}>{message}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
        <DialogFooter className="mt-6">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
          <Button
            type="button"
            variant="primary"
            loading={importMut.isPending}
            disabled={!json.trim()}
            onClick={submit}
          >
            {t("adminAccounts.importSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
