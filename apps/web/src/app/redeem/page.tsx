"use client";

import { useState } from "react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { useRedeemCode } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { meErrorMessage } from "@/lib/me-api";

export default function RedeemPage() {
  return (
    <AppShell allowedRole="user">
      <RedeemContent />
    </AppShell>
  );
}

function RedeemContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const redeemMut = useRedeemCode();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      const result = await redeemMut.mutateAsync({ code: code.trim() });
      if (result.already_redeemed) {
        toast({ title: t("redeem.alreadyRedeemed"), tone: "warning" });
      } else if (result.redemption.type === "balance") {
        toast({
          title: t("redeem.success"),
          description: t("redeem.successBalance", {
            amount: result.redemption.amount,
            currency: result.redemption.currency,
          }),
          tone: "success",
        });
      } else {
        toast({
          title: t("redeem.success"),
          description: t("redeem.successSubscription"),
          tone: "success",
        });
      }
      setCode("");
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAccount")}
        title={t("redeem.title")}
        description={t("redeem.subtitle")}
      />
      <Card className="anim-rise-sm">
        <CardContent>
          <form onSubmit={submit} className="max-w-md space-y-4">
            <div>
              <Label htmlFor="code">{t("redeem.code")}</Label>
              <Input
                id="code"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="SR-XXXX-XXXX"
                className="font-mono"
                autoFocus
                autoComplete="off"
                spellCheck={false}
              />
            </div>
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
            <Button
              type="submit"
              variant="primary"
              loading={redeemMut.isPending}
              disabled={!code.trim()}
            >
              {t("redeem.submit")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
