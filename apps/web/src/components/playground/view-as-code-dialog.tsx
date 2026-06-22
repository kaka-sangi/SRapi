"use client";

import { useMemo, useState } from "react";
import { Code2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "@/components/ui/dialog";
import type { PlaygroundMessage } from "@/lib/playground-client";
import type { PlaygroundParams } from "@/components/playground/playground-settings";
import { useLanguage } from "@/context/LanguageContext";

/** Auth placeholder for the generated snippets — never a real key. The reader
 * substitutes their own gateway key (e.g. via an env var) before running. */
const KEY_PLACEHOLDER = "$SRAPI_API_KEY";

/** Mirrors the base-URL helper the playground-client uses so the generated
 * snippets point at the same gateway the live chat talks to. The OpenAI-
 * compatible surface lives under `/v1` (the playground itself posts to the
 * billed `/api/v1/me/...` route, but a real integration uses the public
 * `/v1/chat/completions` endpoint). */
function apiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

/** OpenAI-compatible chat message: the playground keeps images and reasoning on
 * a turn, but a portable request only needs role + text content. */
interface WireMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

/** Project the current conversation (+ optional system prompt) into the plain
 * `messages` array a real `/v1/chat/completions` call expects. Empty turns and
 * the streaming-only image payloads are dropped. */
function buildWireMessages(messages: PlaygroundMessage[], system: string): WireMessage[] {
  const out: WireMessage[] = [];
  const trimmedSystem = system.trim();
  if (trimmedSystem) out.push({ role: "system", content: trimmedSystem });
  for (const m of messages) {
    const content = (m.content ?? "").trim();
    if (!content) continue;
    out.push({ role: m.role, content });
  }
  // A snippet with no turns yet is still useful as a template.
  if (out.length === 0 || out[out.length - 1].role !== "user") {
    out.push({ role: "user", content: "Hello" });
  }
  return out;
}

/** The OpenAI-compatible request body shared by every snippet variant. Only the
 * params the user actually set are included, matching the playground's "omit to
 * use the model default" behavior. */
function buildRequestBody(
  model: string,
  wire: WireMessage[],
  temperature: number | undefined,
  maxTokens: number | undefined,
): Record<string, unknown> {
  const body: Record<string, unknown> = { model, messages: wire };
  if (temperature !== undefined) body.temperature = temperature;
  if (maxTokens !== undefined) body.max_tokens = maxTokens;
  return body;
}

function curlSnippet(base: string, body: Record<string, unknown>): string {
  // Pretty-print and indent the JSON so the multi-line `-d '...'` stays readable.
  const json = JSON.stringify(body, null, 2)
    .split("\n")
    .map((line, i) => (i === 0 ? line : `  ${line}`))
    .join("\n");
  return `curl ${base}/v1/chat/completions \\
  -H "Authorization: Bearer ${KEY_PLACEHOLDER}" \\
  -H "Content-Type: application/json" \\
  -d '${json}'`;
}

function pythonSnippet(base: string, body: Record<string, unknown>): string {
  const args = Object.entries(body)
    .map(([k, v]) => `    ${k}=${pyValue(v)},`)
    .join("\n");
  return `import os
from openai import OpenAI

client = OpenAI(
    base_url="${base}/v1",
    api_key=os.environ["SRAPI_API_KEY"],
)

resp = client.chat.completions.create(
${args}
)
print(resp.choices[0].message.content)`;
}

function nodeSnippet(base: string, body: Record<string, unknown>): string {
  const json = JSON.stringify(body, null, 2)
    .split("\n")
    .map((line, i) => (i === 0 ? line : `  ${line}`))
    .join("\n");
  return `const resp = await fetch("${base}/v1/chat/completions", {
  method: "POST",
  headers: {
    Authorization: \`Bearer \${process.env.SRAPI_API_KEY}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify(${json}),
});
const data = await resp.json();
console.log(data.choices[0].message.content);`;
}

/** Render a JS value as a Python literal for the SDK kwargs snippet. */
function pyValue(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return JSON.stringify(value, null, 2)
    .split("\n")
    .map((line, i) => (i === 0 ? line : `    ${line}`))
    .join("\n");
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

/**
 * The playground → integration bridge: turns the CURRENT conversation (model +
 * params + messages) into a copy-pasteable real API call against the gateway's
 * OpenAI-compatible `/v1/chat/completions` endpoint. Read-only — it never sends
 * anything and never touches the live chat/streaming state.
 */
export function ViewAsCodeDialog({
  model,
  params,
  messages,
}: {
  model: string;
  params: PlaygroundParams;
  messages: PlaygroundMessage[];
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);

  // Recompute snippets only when the dialog is open and an input actually
  // changed, so opening the panel reflects the latest conversation.
  const snippets = useMemo(() => {
    if (!open) return null;
    const base = apiBaseUrl();
    const temperature = Number.parseFloat(params.temperature);
    const maxTokens = Number.parseInt(params.maxTokens, 10);
    const wire = buildWireMessages(messages, params.system);
    const body = buildRequestBody(
      model,
      wire,
      Number.isFinite(temperature) ? temperature : undefined,
      Number.isFinite(maxTokens) && maxTokens > 0 ? maxTokens : undefined,
    );
    return {
      curl: curlSnippet(base, body),
      python: pythonSnippet(base, body),
      node: nodeSnippet(base, body),
    };
  }, [open, model, params.temperature, params.maxTokens, params.system, messages]);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-8 shrink-0"
          aria-label={t("playground.viewAsCode")}
          title={t("playground.viewAsCode")}
        >
          <Code2 className="size-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="font-sans text-lg font-semibold tracking-tight">
            {t("playground.viewAsCode")}
          </DialogTitle>
          <DialogDescription>{t("playground.viewAsCodeHint")}</DialogDescription>
        </DialogHeader>

        {snippets ? (
          <Tabs defaultValue="curl">
            <TabsList className="w-full overflow-x-auto">
              <TabsTrigger value="curl">cURL</TabsTrigger>
              <TabsTrigger value="python">Python</TabsTrigger>
              <TabsTrigger value="node">Node.js</TabsTrigger>
            </TabsList>
            <TabsContent value="curl" className="mt-3">
              <CodeBlock code={snippets.curl} />
            </TabsContent>
            <TabsContent value="python" className="mt-3">
              <CodeBlock code={snippets.python} />
            </TabsContent>
            <TabsContent value="node" className="mt-3">
              <CodeBlock code={snippets.node} />
            </TabsContent>
          </Tabs>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
