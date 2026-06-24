"use client";

import { useState } from "react";
import { AppShell } from "@/components/layout/app-shell";
import { useRedeemCode } from "@/hooks/queries";
import { SectionHero } from "@/components/visual/section-hero";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { FloatingInput } from "@/components/ui/floating-input";
import { KbdShortcut } from "@/components/ui/kbd";
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
      <SectionHero
        eyebrow={t("redeem.eyebrow")}
        title={t("redeem.title")}
        description={t("redeem.subtitle")}
      />
      <Card className="anim-rise-sm">
        <CardContent>
          <form onSubmit={submit} className="mx-auto max-w-md space-y-5 py-2 text-center">
            <FloatingInput
              id="code"
              label={t("redeem.code")}
              value={code}
              onChange={(v) => {
                if (error) setError(null);
                setCode(v.toUpperCase());
              }}
              placeholder="SR-XXXX-XXXX"
              className="text-center font-mono [&_input]:text-center [&_input]:font-mono [&_input]:tracking-[0.18em] [&_input]:text-lg"
              autoComplete="off"
              error={error ?? undefined}
            />
            <Button
              type="submit"
              variant="primary"
              size="lg"
              className="w-full"
              loading={redeemMut.isPending}
              disabled={!code.trim()}
            >
              {t("redeem.submit")}
            </Button>
            <p className="text-[11px] text-srapi-text-tertiary">
              <KbdShortcut keys={["Enter"]} /> <span className="ml-1">to redeem</span>
            </p>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
