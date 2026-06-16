"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { CheckoutRedirect } from "@/components/features/checkout-redirect";
import { usePaymentOrderStatus } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";

// Landing page payment providers redirect to after Stripe/Alipay/etc. checkout
// completes (the success_url / cancel_url is configured in createOrder). Reads
// ?order_id=, polls the order until it settles, and shows the same provider-
// aware CheckoutRedirect surface used in /billing and /pricing — so if the
// webhook is slightly delayed the user sees a Loader rather than a 404.
export default function PaymentResultPage() {
  return (
    <div className="relative flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-4xl items-center justify-between px-6 py-6">
        <Link href="/" className="font-serif text-2xl leading-none text-srapi-text-primary">
          SRapi
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-md flex-1 items-center justify-center px-6 py-10">
        <Suspense fallback={<LoadingCard />}>
          <ResultContent />
        </Suspense>
      </main>
    </div>
  );
}

function ResultContent() {
  const { t } = useLanguage();
  const params = useSearchParams();
  const orderId = params.get("order_id") || params.get("id") || "";
  const status = usePaymentOrderStatus(orderId || null);

  if (!orderId) {
    return (
      <Card className="card-raised w-full">
        <CardContent className="text-center">
          <h1 className="font-serif text-xl text-srapi-text-primary">
            {t("paymentResult.title")}
          </h1>
          <p className="mt-2 text-sm text-srapi-error">{t("paymentResult.missingOrder")}</p>
          <Link href="/pricing" className="mt-6 block">
            <Button variant="outline" size="lg" className="w-full">
              {t("paymentResult.backToPricing")}
            </Button>
          </Link>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="card-raised w-full">
      <CardContent>
        <h1 className="font-serif text-xl text-srapi-text-primary">
          {t("paymentResult.title")}
        </h1>
        {status.data ? (
          <div className="mt-4">
            <CheckoutRedirect
              order={status.data}
              variant="card"
              successAction={
                <Link href="/billing" className="block">
                  <Button variant="primary" size="lg" className="w-full">
                    {t("paymentResult.backToBilling")}
                  </Button>
                </Link>
              }
              failureAction={
                <Link href="/pricing" className="block">
                  <Button variant="outline" size="lg" className="w-full">
                    {t("paymentResult.backToPricing")}
                  </Button>
                </Link>
              }
            />
          </div>
        ) : (
          <div className="mt-6 flex flex-col items-center gap-3 text-sm text-srapi-text-secondary">
            <Loader2 className="size-6 animate-spin text-srapi-text-tertiary" aria-hidden />
            <span>{t("paymentResult.pendingTitle")}</span>
            <p className="text-2xs text-srapi-text-tertiary">{t("paymentResult.pendingBody")}</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function LoadingCard() {
  return (
    <Card className="card-raised w-full">
      <CardContent>
        <Skeleton className="h-6 w-40" />
        <Skeleton className="mt-6 h-24 w-full" />
      </CardContent>
    </Card>
  );
}
