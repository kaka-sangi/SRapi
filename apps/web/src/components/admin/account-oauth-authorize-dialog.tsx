"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, ExternalLink, Link2, Loader2 } from "lucide-react";
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
import { Label } from "@/components/ui/label";
import { FloatingInput } from "@/components/ui/floating-input";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminApi, adminErrorMessage } from "@/lib/admin-api";
import type {
  AccountOAuthCredential,
  AccountOAuthProviderConfig,
  AccountOAuthDeviceCode,
} from "@/lib/sdk-types";

/** Which provisioning flow the wizard drives, keyed off the runtime class. */
export type AccountOAuthFlowMode = "authorization_code" | "device_code";

/** Tokens lifted out of a completed credential for the parent form to apply. */
export interface ProvisionedTokens {
  accessToken: string;
  refreshToken: string;
}

const DEVICE_POLL_INTERVAL_MS = 2500;
const DEVICE_MAX_POLLS = 120; // ~5 min ceiling regardless of provider expiry

function extractTokens(credential: AccountOAuthCredential): ProvisionedTokens {
  const cred = (credential.credential ?? {}) as Record<string, unknown>;
  return {
    accessToken: typeof cred.access_token === "string" ? cred.access_token : "",
    refreshToken: typeof cred.refresh_token === "string" ? cred.refresh_token : "",
  };
}

/**
 * Interactive OAuth provisioning wizard that mints an upstream-account
 * credential without the operator hand-pasting tokens. For oauth_refresh it
 * runs the authorization-code (PKCE) flow: build a URL, the admin consents in a
 * new tab, then pastes the returned code+state. For oauth_device_code it runs
 * the RFC 8628 device flow: show a user code + verification URI and poll until
 * the provider approves. On success it hands the minted access/refresh tokens
 * back to the account form.
 */
export function AccountOAuthAuthorizeDialog({
  open,
  onOpenChange,
  mode,
  providerId,
  onProvisioned,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: AccountOAuthFlowMode;
  providerId?: string;
  onProvisioned: (tokens: ProvisionedTokens) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();

  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [authorizeUrl, setAuthorizeUrl] = useState("");
  const [tokenUrl, setTokenUrl] = useState("");
  const [deviceAuthorizeUrl, setDeviceAuthorizeUrl] = useState("");
  const [redirectUri, setRedirectUri] = useState("");
  const [scopes, setScopes] = useState("");

  // Authorization-code flow state.
  const [authSessionId, setAuthSessionId] = useState("");
  const [authState, setAuthState] = useState("");
  const [authUrl, setAuthUrl] = useState("");
  const [callbackCode, setCallbackCode] = useState("");

  // Device-code flow state.
  const [device, setDevice] = useState<AccountOAuthDeviceCode | null>(null);
  const [polling, setPolling] = useState(false);

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollCount = useRef(0);
  const prefillKey = useRef("");
  // Holds the latest pollDevice so the self-scheduling setTimeout recursion
  // doesn't reference the callback before its own declaration.
  const pollDeviceRef = useRef<((sessionId: string) => Promise<void>) | null>(null);

  const stopPolling = useCallback(() => {
    if (pollTimer.current) {
      clearTimeout(pollTimer.current);
      pollTimer.current = null;
    }
    pollCount.current = 0;
    setPolling(false);
  }, []);

  const resetAll = useCallback(() => {
    stopPolling();
    setAuthSessionId("");
    setAuthState("");
    setAuthUrl("");
    setCallbackCode("");
    setDevice(null);
    setError(null);
    setBusy(false);
  }, [stopPolling]);

  // Clear transient flow state whenever the dialog closes so a re-open starts
  // fresh (config inputs are intentionally preserved between opens).
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (!open) {
      resetAll();
      prefillKey.current = "";
    }
  }, [open, resetAll]);

  useEffect(() => () => stopPolling(), [stopPolling]);

  useEffect(() => {
    if (!open || !providerId) return;
    const key = `${providerId}:${mode}`;
    if (prefillKey.current === key) return;
    let canceled = false;
    prefillKey.current = key;
    void adminApi
      .getProviderOAuthConfig(providerId)
      .then((result) => {
        if (canceled) return;
        const config = result.config;
        setClientId(config.client_id ?? "");
        setAuthorizeUrl(config.authorize_url ?? "");
        setTokenUrl(config.token_url ?? "");
        setDeviceAuthorizeUrl(config.device_authorize_url ?? "");
        setRedirectUri(config.redirect_uri ?? "");
        setScopes((config.scopes ?? []).join(" "));
      })
      .catch(() => {
        // Providers without preset OAuth metadata still support manual entry.
      });
    return () => {
      canceled = true;
    };
  }, [mode, open, providerId]);

  function buildConfig(): AccountOAuthProviderConfig {
    const scopeList = scopes
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    return {
      client_id: clientId.trim(),
      client_secret: clientSecret.trim() || undefined,
      authorize_url: authorizeUrl.trim() || undefined,
      token_url: tokenUrl.trim() || undefined,
      device_authorize_url: deviceAuthorizeUrl.trim() || undefined,
      redirect_uri: effectiveRedirectUri || undefined,
      scopes: scopeList.length ? scopeList : undefined,
      use_pkce: true,
    };
  }

  function complete(credential: AccountOAuthCredential) {
    const tokens = extractTokens(credential);
    if (!tokens.accessToken && !tokens.refreshToken) {
      setError(t("accountOAuth.errors.noTokens"));
      return;
    }
    onProvisioned(tokens);
    toast({ title: t("accountOAuth.provisioned"), tone: "success" });
    onOpenChange(false);
  }

  async function startAuthorizeUrl() {
    setError(null);
    setBusy(true);
    try {
      const result = await adminApi.startAccountOAuthAuthorizeUrl(buildConfig());
      setAuthSessionId(result.session_id);
      setAuthState(result.state);
      setAuthUrl(result.authorization_url);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function exchangeCode() {
    setError(null);
    setBusy(true);
    try {
      const credential = await adminApi.exchangeAccountOAuthCode({
        sessionId: authSessionId,
        code: callbackCode.trim(),
        callbackUrl: callbackCode.trim(),
        state: authState,
      });
      complete(credential);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const pollDevice = useCallback(
    async (sessionId: string) => {
      try {
        const result = await adminApi.pollAccountOAuthDeviceCode(sessionId);
        if ("status" in result && result.status === "pending") {
          pollCount.current += 1;
          if (pollCount.current >= DEVICE_MAX_POLLS) {
            stopPolling();
            setError(t("accountOAuth.errors.deviceTimeout"));
            return;
          }
          pollTimer.current = setTimeout(
            () => void pollDeviceRef.current?.(sessionId),
            DEVICE_POLL_INTERVAL_MS,
          );
          return;
        }
        stopPolling();
        complete(result as AccountOAuthCredential);
      } catch (err) {
        stopPolling();
        setError(adminErrorMessage(err));
      }
    },
    // complete/t/stopPolling are stable enough for this imperative poller.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [stopPolling, t],
  );
  useEffect(() => {
    pollDeviceRef.current = pollDevice;
  }, [pollDevice]);

  async function startDeviceCode() {
    setError(null);
    setBusy(true);
    try {
      const result = await adminApi.startAccountOAuthDeviceCode(buildConfig());
      setDevice(result);
      setPolling(true);
      pollCount.current = 0;
      pollTimer.current = setTimeout(
        () => void pollDevice(result.session_id),
        DEVICE_POLL_INTERVAL_MS,
      );
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const isAuthCode = mode === "authorization_code";
  const effectiveRedirectUri = redirectUri.trim() || undefined;
  const configReady =
    clientId.trim() !== "" &&
    (isAuthCode
      ? authorizeUrl.trim() !== "" && tokenUrl.trim() !== ""
      : deviceAuthorizeUrl.trim() !== "" && tokenUrl.trim() !== "");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("accountOAuth.title")}</DialogTitle>
          <DialogDescription>
            {isAuthCode ? t("accountOAuth.authCodeIntro") : t("accountOAuth.deviceIntro")}
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
          <div className="rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3.5">
            <button
              type="button"
              onClick={() => setAdvancedOpen((v) => !v)}
              className="flex w-full items-center justify-between text-left"
              aria-expanded={advancedOpen}
            >
              <span className="text-sm font-medium text-srapi-text-primary">
                {t("accountOAuth.advancedConfig")}
              </span>
              <ChevronDown
                className={cn(
                  "size-4 text-srapi-text-tertiary transition-transform",
                  advancedOpen && "rotate-180",
                )}
              />
            </button>
            <p className="mt-1 text-[11px] text-srapi-text-tertiary">
              {t("accountOAuth.advancedConfigHint")}
            </p>
            {advancedOpen ? (
              <div className="mt-3 space-y-3 border-t border-srapi-border pt-3">
                <div>
                  <Label htmlFor="oauth-client-id">{t("accountOAuth.clientId")}</Label>
                  <Input
                    id="oauth-client-id"
                    autoComplete="off"
                    className="mt-1.5 font-mono"
                    value={clientId}
                    disabled={busy || polling}
                    onChange={(e) => setClientId(e.target.value)}
                  />
                </div>
                <div>
                  <Label htmlFor="oauth-client-secret">{t("accountOAuth.clientSecret")}</Label>
                  <Input
                    id="oauth-client-secret"
                    type="password"
                    autoComplete="off"
                    className="mt-1.5 font-mono"
                    value={clientSecret}
                    disabled={busy || polling}
                    onChange={(e) => setClientSecret(e.target.value)}
                  />
                  <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                    {t("accountOAuth.clientSecretHint")}
                  </p>
                </div>
                {isAuthCode ? (
                  <>
                    <div>
                      <Label htmlFor="oauth-authorize-url">{t("accountOAuth.authorizeUrl")}</Label>
                      <Input
                        id="oauth-authorize-url"
                        autoComplete="off"
                        className="mt-1.5 font-mono"
                        value={authorizeUrl}
                        disabled={busy}
                        onChange={(e) => setAuthorizeUrl(e.target.value)}
                      />
                    </div>
                    <div>
                      <Label htmlFor="oauth-redirect-uri">{t("accountOAuth.redirectUri")}</Label>
                      <Input
                        id="oauth-redirect-uri"
                        autoComplete="off"
                        className="mt-1.5 font-mono"
                        value={redirectUri}
                        disabled={busy}
                        onChange={(e) => setRedirectUri(e.target.value)}
                      />
                    </div>
                  </>
                ) : (
                  <div>
                    <Label htmlFor="oauth-device-url">{t("accountOAuth.deviceAuthorizeUrl")}</Label>
                    <Input
                      id="oauth-device-url"
                      autoComplete="off"
                      className="mt-1.5 font-mono"
                      value={deviceAuthorizeUrl}
                      disabled={busy || polling}
                      onChange={(e) => setDeviceAuthorizeUrl(e.target.value)}
                    />
                  </div>
                )}
                <div>
                  <Label htmlFor="oauth-token-url">{t("accountOAuth.tokenUrl")}</Label>
                  <Input
                    id="oauth-token-url"
                    autoComplete="off"
                    className="mt-1.5 font-mono"
                    value={tokenUrl}
                    disabled={busy || polling}
                    onChange={(e) => setTokenUrl(e.target.value)}
                  />
                </div>
                <div>
                  <Label htmlFor="oauth-scopes">{t("accountOAuth.scopes")}</Label>
                  <Input
                    id="oauth-scopes"
                    autoComplete="off"
                    className="mt-1.5 font-mono"
                    placeholder={t("accountOAuth.scopesPlaceholder")}
                    value={scopes}
                    disabled={busy || polling}
                    onChange={(e) => setScopes(e.target.value)}
                  />
                </div>
              </div>
            ) : null}
          </div>

          {isAuthCode ? (
            <div className="space-y-3">
              <div
                className="log-row rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3.5"
                data-sev={authUrl ? "success" : "info"}
              >
                <div className="flex gap-3">
                  <StepBadge value="1" />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-srapi-text-primary">
                      {t("accountOAuth.stepBuildTitle")}
                    </div>
                    <Button
                      type="button"
                      className="mt-2"
                      onClick={() => void startAuthorizeUrl()}
                      disabled={busy || !configReady}
                    >
                      {busy && !authUrl ? <Loader2 className="size-4 animate-spin" /> : <Link2 className="size-4" />}
                      {t("accountOAuth.buildAuthorizeUrl")}
                    </Button>
                  </div>
                </div>
              </div>
              <div
                className="log-row rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3.5"
                data-sev={authUrl ? "info" : "info"}
              >
                <div className="flex gap-3">
                  <StepBadge value="2" />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-srapi-text-primary">
                      {t("accountOAuth.stepOpenTitle")}
                    </div>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("accountOAuth.stepOpenHint")}
                    </p>
                    {authUrl ? (
                      <a
                        href={authUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="mt-2 inline-flex items-center gap-1.5 text-sm text-srapi-primary hover:underline"
                      >
                        <ExternalLink className="size-3.5" />
                        {t("accountOAuth.openAuthorizeUrl")}
                      </a>
                    ) : null}
                  </div>
                </div>
              </div>
              <div
                className="log-row rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3.5"
                data-sev={callbackCode.trim() ? "success" : "info"}
              >
                <div className="flex gap-3">
                  <StepBadge value="3" />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-srapi-text-primary">
                      {t("accountOAuth.stepCodeTitle")}
                    </div>
                    <FloatingInput
                      id="oauth-callback-code"
                      className="mt-2 [&_input]:font-mono"
                      label={t("accountOAuth.callbackCode")}
                      value={callbackCode}
                      onChange={setCallbackCode}
                      disabled={busy || !authUrl}
                      autoComplete="off"
                      hint={t("accountOAuth.pasteCodeHint")}
                    />
                  </div>
                </div>
              </div>
            </div>
          ) : null}

          {/* Device-code: show the user code + verification URI while polling. */}
          {!isAuthCode && device ? (
            <div
              className="log-row space-y-2 rounded-lg border border-srapi-border bg-srapi-card-muted px-3.5 py-3"
              data-sev={polling ? "info" : "success"}
            >
              <p className="text-[11px] text-srapi-text-tertiary">{t("accountOAuth.deviceCodeHint")}</p>
              <div className="flex items-center gap-2">
                <span className="font-mono text-lg tracking-widest text-srapi-text-primary metric-primary">
                  {device.user_code}
                </span>
                <a
                  href={device.verification_uri_complete ?? device.verification_uri}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1.5 text-sm text-srapi-primary hover:underline"
                >
                  <ExternalLink className="size-3.5" />
                  {t("accountOAuth.openVerificationUri")}
                </a>
              </div>
              {polling ? (
                <p className="inline-flex items-center gap-1.5 text-[11px] text-srapi-text-secondary">
                  <Loader2 className="size-3.5 animate-spin" />
                  {t("accountOAuth.waitingForApproval")}
                </p>
              ) : null}
            </div>
          ) : null}

          {error ? (
            <div className="log-row rounded-lg" data-sev="error">
              <p className="px-3 py-2 text-sm text-srapi-error">{error}</p>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
            {t("common.cancel")}
          </Button>
          {isAuthCode ? (
            <Button
              type="button"
              onClick={() => void exchangeCode()}
              disabled={busy || !authUrl || !callbackCode.trim()}
            >
              {busy ? <Loader2 className="size-4 animate-spin" /> : null}
              {t("accountOAuth.completeAuthorization")}
            </Button>
          ) : (
            <Button
              type="button"
              onClick={() => void startDeviceCode()}
              disabled={busy || polling || !configReady}
            >
              {busy || polling ? <Loader2 className="size-4 animate-spin" /> : null}
              {t("accountOAuth.startDeviceCode")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function StepBadge({ value }: { value: string }) {
  return (
    <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-srapi-primary text-sm font-semibold text-white">
      {value}
    </span>
  );
}
