"use client";

import { useState } from "react";
import { CheckCircle2, XCircle, Zap } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton, CopyableValue } from "@/components/ui/copy-button";

const KEY_PLACEHOLDER = "YOUR_API_KEY";

function gatewayOrigin(): string {
  return typeof window === "undefined" ? "" : window.location.origin;
}

function curlSnippet(base: string, key: string, model: string): string {
  return `curl ${base}/v1/chat/completions \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`;
}

function pythonSnippet(base: string, key: string, model: string): string {
  return `from openai import OpenAI

client = OpenAI(base_url="${base}/v1", api_key="${key}")

resp = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "Hello"}],
)
print(resp.choices[0].message.content)`;
}

function nodeSnippet(base: string, key: string, model: string): string {
  return `import OpenAI from "openai";

const client = new OpenAI({ baseURL: "${base}/v1", apiKey: "${key}" });

const resp = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "Hello" }],
});
console.log(resp.choices[0].message.content);`;
}

function claudeCodeSnippet(base: string, key: string): string {
  return `export ANTHROPIC_BASE_URL="${base}"
export ANTHROPIC_AUTH_TOKEN="${key}"
claude`;
}

function CodeBlock({ code }: { code: string }) {
  return (
    <div className="relative rounded-xl border border-srapi-border bg-srapi-card-muted">
      <CopyButton value={code} className="absolute right-1.5 top-1.5 z-10" />
      <pre className="overflow-x-auto p-3 pr-10 font-mono text-xs leading-relaxed text-srapi-text-primary">
        {code}
      </pre>
    </div>
  );
}

type TestState =
  | { phase: "idle" }
  | { phase: "running" }
  | { phase: "ok"; models: number; ms: number }
  | { phase: "fail"; message: string };

/**
 * Quick-start guide for a gateway API key: ready-to-paste snippets (cURL /
 * OpenAI SDKs / Claude Code) with the gateway URL and key inlined, plus a live
 * "test this key" probe against /v1/models. When the plaintext key is not
 * available (existing keys are hashed), an optional paste field inlines it
 * locally — the pasted value never leaves the browser except toward the
 * gateway itself.
 */
export function ApiKeyOnboarding({
  apiKey,
  defaultModel,
  allowPaste = false,
}: {
  /** Plaintext key (creation flow) or null when only the prefix is known. */
  apiKey: string | null;
  /** Model id used in the snippets. */
  defaultModel?: string;
  /** Show a paste field so an existing key can be inlined + tested. */
  allowPaste?: boolean;
}) {
  const { t } = useLanguage();
  const [pasted, setPasted] = useState("");
  const [test, setTest] = useState<TestState>({ phase: "idle" });

  const base = gatewayOrigin();
  const key = apiKey ?? (pasted.trim() || KEY_PLACEHOLDER);
  const model = defaultModel?.trim() || "gpt-4o";
  const canTest = key !== KEY_PLACEHOLDER;

  async function runTest() {
    if (!canTest || test.phase === "running") return;
    setTest({ phase: "running" });
    const started = performance.now();
    try {
      const res = await fetch("/v1/models", {
        headers: { Authorization: `Bearer ${key}` },
      });
      const ms = Math.round(performance.now() - started);
      if (!res.ok) {
        // A 401/403 here means the gateway rejected this key (not an upstream
        // issue) — give an actionable message instead of a bare HTTP code.
        const message =
          res.status === 401 || res.status === 403
            ? t("apiKeys.onboardingTestRejected")
            : `HTTP ${res.status}`;
        setTest({ phase: "fail", message });
        return;
      }
      const body = (await res.json()) as { data?: unknown[] };
      setTest({ phase: "ok", models: body.data?.length ?? 0, ms });
    } catch (err) {
      setTest({
        phase: "fail",
        message: err instanceof Error ? err.message : String(err),
      });
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {t("apiKeys.onboardingTitle")}
        </p>
        <CopyableValue
          value={`${base}/v1`}
          label={t("apiKeys.onboardingBaseUrl")}
          className="max-w-56 text-xs text-srapi-text-secondary"
        />
      </div>

      {allowPaste && !apiKey ? (
        <div>
          <Label htmlFor="onboarding-paste">{t("apiKeys.onboardingPasteLabel")}</Label>
          <Input
            id="onboarding-paste"
            value={pasted}
            onChange={(e) => {
              setPasted(e.target.value);
              setTest({ phase: "idle" });
            }}
            placeholder="sk-…"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
          />
          <p className="mt-1 text-xs text-srapi-text-tertiary">
            {t("apiKeys.onboardingPasteHint")}
          </p>
        </div>
      ) : null}

      <Tabs defaultValue="curl">
        <TabsList className="w-full overflow-x-auto">
          <TabsTrigger value="curl">cURL</TabsTrigger>
          <TabsTrigger value="python">Python</TabsTrigger>
          <TabsTrigger value="node">Node.js</TabsTrigger>
          <TabsTrigger value="claude-code">Claude Code</TabsTrigger>
        </TabsList>
        <TabsContent value="curl" className="mt-2">
          <CodeBlock code={curlSnippet(base, key, model)} />
        </TabsContent>
        <TabsContent value="python" className="mt-2">
          <CodeBlock code={pythonSnippet(base, key, model)} />
        </TabsContent>
        <TabsContent value="node" className="mt-2">
          <CodeBlock code={nodeSnippet(base, key, model)} />
        </TabsContent>
        <TabsContent value="claude-code" className="mt-2">
          <CodeBlock code={claudeCodeSnippet(base, key)} />
          <p className="mt-1.5 text-xs text-srapi-text-tertiary">
            {t("apiKeys.onboardingClaudeCodeHint")}
          </p>
        </TabsContent>
      </Tabs>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canTest}
          loading={test.phase === "running"}
          onClick={() => void runTest()}
        >
          <Zap className="size-3.5" />
          {t("apiKeys.onboardingTest")}
        </Button>
        {test.phase === "ok" ? (
          <span className="inline-flex items-center gap-1 text-xs text-srapi-success anim-pop-in">
            <CheckCircle2 className="size-3.5" aria-hidden />
            {t("apiKeys.onboardingTestOk", { count: test.models, ms: test.ms })}
          </span>
        ) : null}
        {test.phase === "fail" ? (
          <span role="alert" className="inline-flex items-center gap-1 text-xs text-srapi-error">
            <XCircle className="size-3.5" aria-hidden />
            {t("apiKeys.onboardingTestFail", { message: test.message })}
          </span>
        ) : null}
      </div>
    </div>
  );
}
