"use client";

import { useState } from "react";
import { KeyRound, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useLanguage } from "@/context/LanguageContext";

export type OAuthSubMethod = "oauth" | "setup-token" | "refresh-token";

interface OAuthInputProps {
  platform: string;
  disabled?: boolean;
  onAuthorize: () => void;
  onCredential: (cred: Record<string, unknown>, runtimeClassOverride?: string) => void;
}

export function OAuthInput({ platform, disabled, onAuthorize, onCredential }: OAuthInputProps) {
  const { t } = useLanguage();

  const subMethods: { value: OAuthSubMethod; label: string }[] =
    platform === "anthropic"
      ? [
          { value: "oauth", label: t("adminAccounts.oauthAuthorize") },
          { value: "setup-token", label: t("adminAccounts.setupToken") },
        ]
      : platform === "openai"
        ? [
            { value: "oauth", label: t("adminAccounts.oauthAuthorize") },
            { value: "refresh-token", label: "Refresh Token" },
          ]
        : [{ value: "oauth", label: t("adminAccounts.oauthAuthorize") }];

  const [method, setMethod] = useState<OAuthSubMethod>("oauth");
  const [tokenInput, setTokenInput] = useState("");

  function handleApply() {
    const value = tokenInput.trim();
    if (!value) return;
    if (method === "setup-token") {
      onCredential({ cli_client_token: value }, "cli_client_token");
    } else {
      onCredential({ refresh_token: value });
    }
  }

  return (
    <div className="space-y-4">
      {subMethods.length > 1 ? (
        <div>
          <Label>{t("adminAccounts.addMethod")}</Label>
          <div className="mt-2 flex gap-4">
            {subMethods.map((m) => (
              <label key={m.value} className="flex cursor-pointer items-center gap-2">
                <input
                  type="radio"
                  name="oauth-method"
                  value={m.value}
                  checked={method === m.value}
                  onChange={() => setMethod(m.value)}
                  className="text-srapi-primary"
                />
                <span className="text-sm">{m.label}</span>
              </label>
            ))}
          </div>
        </div>
      ) : null}

      {method === "oauth" ? (
        <div className="rounded-lg border border-srapi-primary/20 bg-srapi-primary/5 p-4">
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            onClick={onAuthorize}
            className="w-full"
          >
            <KeyRound className="size-3.5" />
            {t("accountOAuth.authorizeAccount")}
          </Button>
          <p className="mt-2 text-center text-[11px] text-srapi-text-tertiary">
            {t("adminAccounts.oauthAuthorizeHint")}
          </p>
        </div>
      ) : method === "setup-token" ? (
        <div>
          <Label htmlFor="setup-token-input">{t("adminAccounts.setupTokenLabel")}</Label>
          <Input
            id="setup-token-input"
            type="password"
            className="mt-1.5 font-mono"
            placeholder={t("adminAccounts.setupTokenPlaceholder")}
            value={tokenInput}
            disabled={disabled}
            autoComplete="new-password"
            data-lpignore="true"
            onChange={(e) => setTokenInput(e.target.value)}
          />
          <p className="mt-1 text-[11px] text-srapi-text-tertiary">
            {t("adminAccounts.setupTokenHint")}
          </p>
        </div>
      ) : (
        <div>
          <Label htmlFor="refresh-token-input">Refresh Token</Label>
          <Textarea
            id="refresh-token-input"
            spellCheck={false}
            className="mt-1.5 min-h-20 font-mono text-xs"
            placeholder={t("adminAccounts.refreshTokenPlaceholder")}
            value={tokenInput}
            disabled={disabled}
            autoComplete="off"
            data-lpignore="true"
            onChange={(e) => setTokenInput(e.target.value)}
          />
          <div className="mt-2 flex justify-end">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled || !tokenInput.trim()}
              onClick={handleApply}
            >
              <Sparkles className="size-3.5" />
              {t("accountOAuth.quickPasteApply")}
            </Button>
          </div>
          <p className="mt-1 text-[11px] text-srapi-text-tertiary">
            {t("adminAccounts.refreshTokenHint")}
          </p>
        </div>
      )}
    </div>
  );
}
