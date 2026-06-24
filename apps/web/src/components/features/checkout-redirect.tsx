"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { QRCodeSVG } from "qrcode.react";
import { CheckCircle2, ExternalLink, Loader2, Smartphone, XCircle } from "lucide-react";
import { usePaymentOrderStatus } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney } from "@/lib/admin-format";
import type { PaymentOrder } from "@/lib/sdk-types";

/**
 * Renders a checkout call-to-action whose shape depends on the payment
 * provider/method. The provider's intent is encoded in order.provider_snapshot
 * (set in apps/api/internal/modules/payments/service/service.go) and in the
 * concrete url scheme:
 *
 *   - Stripe (HTTPS https://checkout.stripe.com/…) → "Go to payment" external link.
 *   - Alipay (HTTPS https://openapi.alipay.com/…) → external link, opens the
 *     hosted page-pay form.
 *   - WeChat Native (weixin://wxpay/…) → renders the URL as a QR code; the
 *     scheme is a deep link that is dead on desktop unless scanned with the
 *     WeChat app camera.
 *   - WeChat H5/JSAPI → on mobile we can deep-link; on desktop we still QR.
 *   - Anything else with an HTTPS URL → external link fallback.
 *
 * The component polls the order while pending (3s, via usePaymentOrderStatus)
 * and switches to settled/failed UI when the webhook lands.
 */
type CheckoutRedirectVariant = "inline" | "card";

export interface CheckoutRedirectProps {
  order: PaymentOrder;
  variant?: CheckoutRedirectVariant;
  // Invalidate caches that depend on the user's money state once the payment
  // settles — defaults to balance + orders + subscriptions.
  invalidateOnSettle?: readonly (readonly string[])[];
  // Slot rendered below the call-to-action on success (e.g. "View billing").
  successAction?: React.ReactNode;
  // Slot rendered below the call-to-action on failure (e.g. "Try again").
  failureAction?: React.ReactNode;
}

const DEFAULT_INVALIDATE = [
  ["me", "balance"],
  ["me", "orders"],
  ["me", "subscriptions"],
] as const;

export function CheckoutRedirect({
  order,
  variant = "inline",
  invalidateOnSettle = DEFAULT_INVALIDATE,
  successAction,
  failureAction,
}: CheckoutRedirectProps) {
  const { t } = useLanguage();
  const qc = useQueryClient();
  const status = usePaymentOrderStatus(order.id);
  const live = status.data ?? order;
  const settled = live.status === "paid" || live.status === "fulfilled";
  const failed =
    live.status === "canceled" || live.status === "expired" || live.status === "failed";

  useEffect(() => {
    if (!settled) return;
    for (const key of invalidateOnSettle) {
      void qc.invalidateQueries({ queryKey: [...key] });
    }
  }, [settled, qc, invalidateOnSettle]);

  const containerClass =
    variant === "card"
      ? "rounded-xl border border-srapi-border bg-srapi-card px-4 py-3 text-sm"
      : "text-sm";

  return (
    <div className={containerClass}>
      <dl>
        <div className="flex items-center justify-between">
          <dt className="text-srapi-text-tertiary">{t("billing.orderNo")}</dt>
          <dd className="font-mono text-xs text-srapi-text-secondary">{live.order_no}</dd>
        </div>
        <div className="mt-1.5 flex items-center justify-between">
          <dt className="text-srapi-text-tertiary">{t("billing.payable")}</dt>
          <dd className="text-base font-semibold tracking-tight tabular text-srapi-text-primary">
            {formatMoney(live.payable_amount, live.currency)}
          </dd>
        </div>
      </dl>

      {settled ? (
        <div className="mt-3 space-y-3">
          <div className="anim-pop-in flex items-center gap-2 text-srapi-success">
            <CheckCircle2 className="size-4 shrink-0" aria-hidden />
            <span>{t("billing.paymentConfirmed")}</span>
          </div>
          {successAction}
        </div>
      ) : failed ? (
        <div className="mt-3 space-y-3">
          <div className="flex items-center gap-2 text-srapi-error">
            <XCircle className="size-4 shrink-0" aria-hidden />
            <span>
              <QuietBadge status={quietStatusFor(live.status)} label={statusLabel(t, live.status)} />
            </span>
          </div>
          {failureAction}
        </div>
      ) : live.status === "pending" ? (
        <PendingCheckout order={live} />
      ) : (
        <div className="mt-3">
          <QuietBadge status={quietStatusFor(live.status)} label={statusLabel(t, live.status)} />
        </div>
      )}
    </div>
  );
}

function PendingCheckout({ order }: { order: PaymentOrder }) {
  const { t } = useLanguage();
  const shape = detectCheckoutShape(order);

  if (shape.kind === "none") {
    return (
      <div className="mt-3 inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
        <Loader2 className="size-3.5 animate-spin" aria-hidden />
        {t("billing.waitingPayment")}
      </div>
    );
  }

  if (shape.kind === "external") {
    return <ExternalCheckout url={shape.url} autoRedirect={shape.autoRedirect} />;
  }

  if (shape.kind === "qr") {
    return (
      <div className="mt-3 flex flex-col items-center gap-2 rounded-xl border border-srapi-border bg-srapi-card-muted px-4 py-4">
        <QRCodeSVG value={shape.url} size={192} level="M" includeMargin />
        <p className="mt-1 text-xs text-srapi-text-secondary">
          {shape.providerLabel
            ? t("checkout.scanWith", { app: shape.providerLabel })
            : t("checkout.scanGeneric")}
        </p>
        <span className="inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
          <Loader2 className="size-3.5 animate-spin" aria-hidden />
          {t("billing.waitingPayment")}
        </span>
      </div>
    );
  }

  // Deep link (WeChat H5). Show both: the button (for mobile WeChat in-app
  // browser) AND a QR fallback for desktop, since the same URL is useless
  // outside of WeChat on desktop.
  return (
    <div className="mt-3 space-y-3">
      <Button asChild variant="primary" size="sm">
        <a href={shape.url}>
          <Smartphone className="size-3.5" />
          {t("checkout.openInApp")}
        </a>
      </Button>
      <div className="flex flex-col items-center gap-2 rounded-xl border border-srapi-border bg-srapi-card-muted px-4 py-4">
        <QRCodeSVG value={shape.url} size={160} level="M" includeMargin />
        <p className="text-xs text-srapi-text-secondary">
          {t("checkout.scanGeneric")}
        </p>
      </div>
      <span className="inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
        <Loader2 className="size-3.5 animate-spin" aria-hidden />
        {t("billing.waitingPayment")}
      </span>
    </div>
  );
}

function ExternalCheckout({ url, autoRedirect }: { url: string; autoRedirect?: boolean }) {
  const { t } = useLanguage();
  useEffect(() => {
    if (autoRedirect) {
      window.location.href = url;
    }
  }, [autoRedirect, url]);

  if (autoRedirect) {
    return (
      <div className="mt-3 inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
        <Loader2 className="size-3.5 animate-spin" aria-hidden />
        {t("billing.redirectingPayment")}
      </div>
    );
  }

  return (
    <div className="mt-3 flex flex-wrap items-center gap-3">
      <Button asChild variant="primary" size="sm">
        <a href={url} target="_blank" rel="noopener noreferrer">
          <ExternalLink className="size-3.5" />
          {t("billing.goPay")}
        </a>
      </Button>
      <span className="inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
        <Loader2 className="size-3.5 animate-spin" aria-hidden />
        {t("billing.waitingPayment")}
      </span>
    </div>
  );
}

type CheckoutShape =
  | { kind: "none" }
  | { kind: "external"; url: string; autoRedirect?: boolean }
  | { kind: "qr"; url: string; providerLabel?: string }
  | { kind: "deeplink"; url: string };

// Picks the right rendering shape from what the backend stored in order
// metadata + provider_snapshot. Centralised so /billing, /pricing and
// /payment/result agree on the rule.
function detectCheckoutShape(order: PaymentOrder): CheckoutShape {
  const meta = (order.metadata ?? {}) as Record<string, unknown>;
  const snap = (order.provider_snapshot ?? {}) as Record<string, unknown>;
  const provider = typeof snap.provider === "string" ? snap.provider.toLowerCase() : "";
  const method = typeof snap.method === "string" ? snap.method.toLowerCase() : "";
  const url = typeof meta.checkout_url === "string" ? meta.checkout_url : "";
  if (!url) return { kind: "none" };

  // WeChat Native: the CodeUrl is the literal QR payload (weixin://wxpay/…)
  // and only resolves inside the WeChat app's camera.
  if (provider === "wechat") {
    if (url.startsWith("weixin://")) {
      return { kind: "qr", url, providerLabel: "WeChat" };
    }
    if (/^(jsapi|h5)$/.test(method) || url.startsWith("http")) {
      // H5 returns an https mweb_url that deep-links into WeChat on mobile;
      // JSAPI is invoked inside the WeChat in-app browser.
      return { kind: "deeplink", url };
    }
    return { kind: "qr", url, providerLabel: "WeChat" };
  }

  // Alipay returns an https form URL (page pay) or alipays:// scheme on mobile.
  if (provider === "alipay") {
    if (url.startsWith("alipays://") || url.startsWith("alipayqr://")) {
      return { kind: "qr", url, providerLabel: "Alipay" };
    }
    return { kind: "external", url };
  }

  // EasyPay / Linux.do Credit: auto-redirect to the payment page (same-window
  // form submission) — the return_url brings the user back after payment.
  if (provider === "linuxdo" || provider === "easypay") {
    return { kind: "external", url, autoRedirect: true };
  }

  // Stripe + everything else with an HTTPS checkout: open the hosted page.
  return { kind: "external", url };
}
