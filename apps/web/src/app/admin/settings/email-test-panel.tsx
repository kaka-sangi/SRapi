import { useState } from "react";
import { CheckCircle2, XCircle, Loader2, Mail } from "lucide-react";
import { useSendTestEmail } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminErrorMessage } from "@/lib/admin-api";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/admin-format";

/**
 * "Send test email" control for the email tab. The SMTP password is write-only,
 * so a live delivery is the only way to confirm the saved credentials work. Save
 * the SMTP fields first, then send to the admin's own mailbox (or an override)
 * and read the per-step checks below.
 */
export function EmailTestPanel() {
  const { t } = useLanguage();
  const sendMut = useSendTestEmail();
  const [recipient, setRecipient] = useState("");
  const result = sendMut.data;
  const loading = sendMut.isPending;
  const failed = !loading && (sendMut.isError || result?.ok === false);
  const ok = !loading && !sendMut.isError && result?.ok === true;
  const checks = (result?.checks as Record<string, unknown> | undefined) ?? undefined;

  function send() {
    const trimmed = recipient.trim();
    sendMut.mutate(trimmed ? { recipient: trimmed } : undefined);
  }

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex items-center gap-2">
        <Mail className="size-4 text-srapi-text-tertiary" />
        <span className="text-sm font-medium text-srapi-text-primary">
          {t("adminSettings.testEmail.title")}
        </span>
      </div>
      <p className="mt-1 text-2xs text-srapi-text-tertiary">
        {t("adminSettings.testEmail.hint")}
      </p>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end">
        <div className="flex-1">
          <Label htmlFor="s-test-email-recipient">
            {t("adminSettings.testEmail.recipient")}
          </Label>
          <Input
            id="s-test-email-recipient"
            type="email"
            className="mt-1.5"
            value={recipient}
            placeholder={t("adminSettings.testEmail.recipientPlaceholder")}
            onChange={(e) => setRecipient(e.target.value)}
          />
        </div>
        <Button variant="outline" loading={loading} onClick={send}>
          {t("adminSettings.testEmail.send")}
        </Button>
      </div>

      {loading || result || sendMut.isError ? (
        <div className="mt-3 rounded-lg border border-srapi-border bg-srapi-card p-3.5 font-mono text-xs">
          <div className="flex items-center gap-2">
            {loading ? (
              <>
                <Loader2 className="size-3.5 animate-spin text-srapi-text-tertiary" />
                <span className="text-srapi-text-secondary">
                  {t("adminSettings.testEmail.running")}
                </span>
              </>
            ) : failed ? (
              <>
                <XCircle className="size-3.5 text-srapi-error" />
                <span className="text-srapi-error">{t("adminSettings.testEmail.failed")}</span>
              </>
            ) : ok ? (
              <>
                <CheckCircle2 className="size-3.5 text-srapi-success" />
                <span className="text-srapi-success">{t("adminSettings.testEmail.ok")}</span>
              </>
            ) : null}
          </div>

          {!loading && (sendMut.isError || result?.message) ? (
            <p className="mt-2 break-words text-srapi-text-secondary">
              {sendMut.isError ? adminErrorMessage(sendMut.error) : result?.message}
            </p>
          ) : null}

          {!loading && checks && Object.keys(checks).length > 0 ? (
            <dl className="mt-2.5 space-y-1 border-t border-srapi-border pt-2.5">
              {Object.entries(checks).map(([k, v]) => (
                <div key={k} className="flex items-baseline justify-between gap-3">
                  <dt className="text-srapi-text-tertiary">{k}</dt>
                  <dd
                    className={cn(
                      "tabular text-right",
                      v === true
                        ? "text-srapi-success"
                        : v === false
                          ? "text-srapi-error"
                          : "text-srapi-text-primary",
                    )}
                  >
                    {stringifyEmailCheck(v)}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}

          {!loading && result?.checked_at ? (
            <p className="mt-2.5 text-2xs text-srapi-text-tertiary">
              {formatDateTime(result.checked_at)}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function stringifyEmailCheck(value: unknown): string {
  if (typeof value === "boolean") return value ? "✓" : "✗";
  if (value == null) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
