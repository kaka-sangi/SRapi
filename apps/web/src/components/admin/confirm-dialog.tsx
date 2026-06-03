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
import { Input } from "@/components/ui/input";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";

/**
 * Confirmation dialog for destructive / high-risk actions. When `confirmPhrase`
 * is set the confirm button stays disabled until the operator types it verbatim
 * — driving both simple deletes and the typed-phrase guards already defined in
 * the lib/admin-*-form helpers (e.g. delete announcement = its title).
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirmLabel,
  tone = "danger",
  confirmPhrase,
  onConfirm,
  successMessage,
  isPending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  body?: string;
  confirmLabel?: string;
  tone?: "danger" | "default";
  confirmPhrase?: string;
  onConfirm: () => Promise<unknown>;
  successMessage?: string;
  isPending?: boolean;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const [phrase, setPhrase] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const canConfirm = !confirmPhrase || phrase.trim() === confirmPhrase;
  const busy = submitting || Boolean(isPending);

  async function confirm() {
    setError(null);
    setSubmitting(true);
    try {
      await onConfirm();
      if (successMessage) toast({ title: successMessage, tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {body ? <DialogDescription>{body}</DialogDescription> : null}
        </DialogHeader>
        {confirmPhrase ? (
          <div className="mt-2">
            <p className="mb-2 text-sm text-srapi-text-secondary">
              {t("feedback.typeToConfirm", { phrase: confirmPhrase })}
            </p>
            <Input
              value={phrase}
              autoFocus
              disabled={busy}
              onChange={(event) => setPhrase(event.target.value)}
            />
          </div>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-srapi-error">
            {error}
          </p>
        ) : null}
        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="ghost"
            disabled={busy}
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant={tone === "danger" ? "danger" : "primary"}
            disabled={!canConfirm}
            loading={busy}
            onClick={confirm}
          >
            {confirmLabel ?? t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
