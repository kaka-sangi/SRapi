"use client";

import { useEffect, useRef, useState } from "react";
import { apiService } from "@/lib/api";

/**
 * Human-verification widget. Renders the configured provider's challenge and
 * hands the resulting token up via onToken (called with "" when it expires or
 * errors). Driven entirely by the server's captcha config (provider + site key),
 * so enabling Cloudflare Turnstile on the backend makes this appear with no
 * frontend redeploy. Turnstile / hCaptcha / reCAPTCHA share a near-identical
 * explicit-render API: global.render(el, { sitekey, callback }).
 */

type ProviderMeta = { script: string; global: string };

const PROVIDERS: Record<string, ProviderMeta> = {
  turnstile: {
    script: "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit",
    global: "turnstile",
  },
  hcaptcha: {
    script: "https://js.hcaptcha.com/1/api.js?render=explicit",
    global: "hcaptcha",
  },
  recaptcha: {
    script: "https://www.google.com/recaptcha/api.js?render=explicit",
    global: "grecaptcha",
  },
};

type WidgetApi = {
  render: (el: HTMLElement, opts: Record<string, unknown>) => string;
  remove?: (id: string) => void;
  reset?: (id?: string) => void;
};

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    if (document.querySelector(`script[src="${src}"]`)) {
      resolve();
      return;
    }
    const el = document.createElement("script");
    el.src = src;
    el.async = true;
    el.defer = true;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error("captcha script failed to load"));
    document.head.appendChild(el);
  });
}

/** Poll for the provider's global to expose render() (scripts attach it async). */
function waitForWidget(globalName: string, timeoutMs = 8000): Promise<WidgetApi> {
  return new Promise((resolve, reject) => {
    const start = performance.now();
    const tick = () => {
      const api = (window as unknown as Record<string, WidgetApi | undefined>)[globalName];
      if (api && typeof api.render === "function") {
        resolve(api);
        return;
      }
      if (performance.now() - start > timeoutMs) {
        reject(new Error("captcha widget unavailable"));
        return;
      }
      requestAnimationFrame(tick);
    };
    tick();
  });
}

export function Captcha({
  provider,
  siteKey,
  onToken,
  className,
}: {
  provider: string;
  siteKey: string;
  onToken: (token: string) => void;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  // Keep the latest onToken without re-rendering the widget on every parent render.
  const onTokenRef = useRef(onToken);
  onTokenRef.current = onToken;

  const meta = PROVIDERS[provider];

  useEffect(() => {
    if (!meta || !siteKey || !ref.current) return;
    let cancelled = false;
    let widgetId: string | null = null;
    let api: WidgetApi | null = null;

    loadScript(meta.script)
      .then(() => waitForWidget(meta.global))
      .then((widget) => {
        if (cancelled || !ref.current) return;
        api = widget;
        widgetId = widget.render(ref.current, {
          sitekey: siteKey,
          callback: (token: string) => onTokenRef.current(token),
          "expired-callback": () => onTokenRef.current(""),
          "error-callback": () => onTokenRef.current(""),
        });
      })
      .catch(() => {
        /* network/script failure — leave the reserved space empty */
      });

    return () => {
      cancelled = true;
      if (widgetId && api?.remove) {
        try {
          api.remove(widgetId);
        } catch {
          /* provider already torn down */
        }
      }
    };
  }, [meta, siteKey]);

  if (!meta || !siteKey) return null;
  // Reserve vertical space so the form doesn't jump when the challenge mounts.
  return <div ref={ref} className={className ?? "min-h-[68px]"} />;
}

type CaptchaConfig = { enabled: boolean; provider: string; site_key: string };
const CAPTCHA_OFF: CaptchaConfig = { enabled: false, provider: "turnstile", site_key: "" };

/**
 * Fetches the server's captcha config once and, when enabled, yields a ready-to-
 * drop-in widget node + the live token. `required` lets a form block submit until
 * the challenge is solved. Treats any fetch failure as "captcha off" so a config
 * hiccup never locks users out of a captcha-less deployment.
 */
export function useCaptcha() {
  const [config, setConfig] = useState<CaptchaConfig | null>(null);
  const [token, setToken] = useState("");

  useEffect(() => {
    let active = true;
    apiService
      .getCaptchaConfig()
      .then((c) => {
        if (active) setConfig(c ?? CAPTCHA_OFF);
      })
      .catch(() => {
        if (active) setConfig(CAPTCHA_OFF);
      });
    return () => {
      active = false;
    };
  }, []);

  const required = Boolean(config?.enabled && config.site_key);
  const node =
    required && config ? (
      <Captcha provider={config.provider} siteKey={config.site_key} onToken={setToken} />
    ) : null;

  return { required, token, node };
}
