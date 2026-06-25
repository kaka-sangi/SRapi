"use client";

import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";

const BEDROCK_REGIONS = [
  "us-east-1", "us-east-2", "us-west-2",
  "eu-west-1", "eu-west-3", "eu-central-1",
  "ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
  "ca-central-1", "sa-east-1",
];

interface BedrockInputProps {
  disabled?: boolean;
  onCredential: (cred: Record<string, unknown>) => void;
}

export function BedrockInput({ disabled, onCredential }: BedrockInputProps) {
  const { t } = useLanguage();
  const [authMode, setAuthMode] = useState<"sigv4" | "apikey">("sigv4");
  const [accessKeyId, setAccessKeyId] = useState("");
  const [secretAccessKey, setSecretAccessKey] = useState("");
  const [sessionToken, setSessionToken] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const [apiKey, setApiKey] = useState("");
  const [showSecret, setShowSecret] = useState(false);

  function emitCredential() {
    if (authMode === "sigv4") {
      onCredential({
        auth_mode: "sigv4",
        aws_access_key_id: accessKeyId.trim(),
        aws_secret_access_key: secretAccessKey.trim(),
        aws_session_token: sessionToken.trim() || undefined,
        aws_region: region,
      });
    } else {
      onCredential({
        auth_mode: "api_key",
        api_key: apiKey.trim(),
        aws_region: region,
      });
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <Label>{t("adminAccounts.bedrock.authMode")}</Label>
        <div className="mt-2 flex gap-4">
          <label className="flex cursor-pointer items-center gap-2">
            <input type="radio" name="bedrock-auth" value="sigv4" checked={authMode === "sigv4"} onChange={() => setAuthMode("sigv4")} className="text-srapi-primary" />
            <span className="text-sm">SigV4 (IAM)</span>
          </label>
          <label className="flex cursor-pointer items-center gap-2">
            <input type="radio" name="bedrock-auth" value="apikey" checked={authMode === "apikey"} onChange={() => setAuthMode("apikey")} className="text-srapi-primary" />
            <span className="text-sm">API Key</span>
          </label>
        </div>
      </div>

      <div>
        <Label>{t("adminAccounts.bedrock.region")}</Label>
        <Select value={region} onValueChange={setRegion} disabled={disabled}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            {BEDROCK_REGIONS.map((r) => (
              <SelectItem key={r} value={r}>{r}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {authMode === "sigv4" ? (
        <>
          <div>
            <Label htmlFor="bedrock-access-key">Access Key ID</Label>
            <Input id="bedrock-access-key" className="mt-1.5 font-mono" placeholder="AKIA..." value={accessKeyId} disabled={disabled} onChange={(e) => { setAccessKeyId(e.target.value); emitCredential(); }} />
          </div>
          <div>
            <Label htmlFor="bedrock-secret-key">Secret Access Key</Label>
            <div className="relative mt-1.5">
              <Input id="bedrock-secret-key" type={showSecret ? "text" : "password"} className="pr-9 font-mono" value={secretAccessKey} disabled={disabled} autoComplete="new-password" onChange={(e) => { setSecretAccessKey(e.target.value); emitCredential(); }} />
              <button type="button" tabIndex={-1} onClick={() => setShowSecret((v) => !v)} className="absolute right-2 top-1/2 -translate-y-1/2 text-srapi-text-tertiary">
                {showSecret ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
              </button>
            </div>
          </div>
          <div>
            <Label htmlFor="bedrock-session-token">{t("adminAccounts.bedrock.sessionToken")}</Label>
            <Input id="bedrock-session-token" className="mt-1.5 font-mono" placeholder={t("adminAccounts.bedrock.sessionTokenHint")} value={sessionToken} disabled={disabled} onChange={(e) => { setSessionToken(e.target.value); emitCredential(); }} />
          </div>
        </>
      ) : (
        <div>
          <Label htmlFor="bedrock-api-key">API Key</Label>
          <div className="relative mt-1.5">
            <Input id="bedrock-api-key" type={showSecret ? "text" : "password"} className="pr-9 font-mono" value={apiKey} disabled={disabled} autoComplete="new-password" onChange={(e) => { setApiKey(e.target.value); emitCredential(); }} />
            <button type="button" tabIndex={-1} onClick={() => setShowSecret((v) => !v)} className="absolute right-2 top-1/2 -translate-y-1/2 text-srapi-text-tertiary">
              {showSecret ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
