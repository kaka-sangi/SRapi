"use client";

import * as React from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Key, Plus, Copy, Check, AlertCircle, Power, Sparkles } from "lucide-react";
import DashboardLayout from "@/components/DashboardLayout";
import { useApiKeys, useCreateApiKey, useToggleApiKey } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import {
  createApiKeySchema,
  parseGroupIdsCsv,
  type CreateApiKeyValues,
} from "@/lib/schemas/api-key";
import type { MockApiKey } from "@/lib/mockData";

const DEFAULT_MODELS: readonly string[] = [
  "gpt-4o-mini",
  "claude-3-5-sonnet",
  "gemini-1.5-pro",
  "gemini-1.5-flash",
];

export default function ApiKeysPage() {
  const { language, t } = useLanguage();
  const apiKeysQuery = useApiKeys();
  const createMutation = useCreateApiKey();
  const toggleMutation = useToggleApiKey();
  const keys = apiKeysQuery.data ?? [];
  const loading = apiKeysQuery.isLoading;

  const [copiedId, setCopiedId] = React.useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = React.useState(false);
  const [generatedKey, setGeneratedKey] = React.useState<MockApiKey | null>(null);
  const [copiedPlaintext, setCopiedPlaintext] = React.useState(false);
  const [groupsCsv, setGroupsCsv] = React.useState("group-01");

  const form = useForm<CreateApiKeyValues>({
    resolver: zodResolver(createApiKeySchema),
    defaultValues: {
      name: "",
      allowedModels: ["gpt-4o-mini"],
      groupIds: ["group-01"],
    },
  });
  const selectedModels =
    useWatch({
      control: form.control,
      name: "allowedModels",
    }) ?? [];

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    window.setTimeout(() => setCopiedId(null), 1500);
  };

  const handleCopyPlaintext = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedPlaintext(true);
    window.setTimeout(() => setCopiedPlaintext(false), 1500);
  };

  const onSubmit = form.handleSubmit(async (values) => {
    const newKey = await createMutation.mutateAsync({
      name: values.name,
      allowedModels: values.allowedModels,
      groupIds: values.groupIds,
    });
    setGeneratedKey(newKey);
    form.reset({ name: "", allowedModels: ["gpt-4o-mini"], groupIds: ["group-01"] });
    setGroupsCsv("group-01");
    setShowCreateModal(false);
  });

  const handleToggleStatus = async (id: string, currentStatus: "active" | "disabled") => {
    await toggleMutation.mutateAsync({ id, currentStatus });
  };

  const handleModelToggle = (model: string) => {
    const current = form.getValues("allowedModels");
    const next = current.includes(model)
      ? current.filter((m) => m !== model)
      : [...current, model];
    form.setValue("allowedModels", next, { shouldValidate: true, shouldDirty: true });
  };

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textHeading = language === "en" ? "API keys" : "API 密钥";
  const textHeadingDesc =
    language === "en"
      ? "Issue scoped API keys for your apps. SRapi only stores an HMAC hash, so the secret is shown once at creation."
      : "为应用颁发带作用域的 API 密钥。SRapi 只保存 HMAC 哈希，明文仅在创建时显示一次。";
  const textActiveBadge = language === "en" ? "Active" : "启用";
  const textDisabledBadge = language === "en" ? "Disabled" : "已停用";
  const placeholderName = language === "en" ? "e.g. production-web" : "例如：production-web";

  return (
    <DashboardLayout>
      <div className="space-y-8 animate-bloom">
        {/* Top operational header */}
        <div className="tactile-card flex flex-col justify-between gap-6 rounded-2xl border border-srapi-border bg-srapi-card p-6 sm:flex-row sm:items-center">
          <div className="space-y-1">
            <h3 className="font-serif text-lg font-medium tracking-tight">{textHeading}</h3>
            <p className="text-xs leading-relaxed text-srapi-text-secondary">{textHeadingDesc}</p>
          </div>
          <Button
            onClick={() => {
              setGeneratedKey(null);
              setShowCreateModal(true);
            }}
            size="md"
          >
            <Plus size={14} aria-hidden="true" />
            {t("generateKey")}
          </Button>
        </div>

        {/* One-time plaintext display */}
        {generatedKey && generatedKey.plaintextKey ? (
          <div className="space-y-4 rounded-2xl border border-srapi-primary/30 bg-srapi-primary/5 p-6 animate-bloom">
            <div className="flex items-start gap-3.5">
              <AlertCircle
                size={18}
                aria-hidden="true"
                className="mt-0.5 shrink-0 text-srapi-primary"
              />
              <div className="space-y-1">
                <h4 className="font-mono text-xs font-extrabold uppercase tracking-wider text-srapi-primary">
                  {t("secretKeyGenerated")}
                </h4>
                <p className="text-xs leading-relaxed text-srapi-text-secondary">
                  {t("keyWarning")}
                </p>
              </div>
            </div>

            <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
              <code className="block flex-grow select-all overflow-x-auto rounded-xl border border-srapi-border bg-srapi-card p-3.5 font-mono text-xs font-bold text-srapi-text-primary">
                {generatedKey.plaintextKey}
              </code>
              <Button
                variant="accent"
                onClick={() => handleCopyPlaintext(generatedKey.plaintextKey!)}
              >
                {copiedPlaintext ? (
                  <Check size={14} aria-hidden="true" />
                ) : (
                  <Copy size={14} aria-hidden="true" />
                )}
                {copiedPlaintext ? t("copiedClipboard") : t("copyPlaintext")}
              </Button>
            </div>
          </div>
        ) : null}

        {/* Keys table */}
        <div className="tactile-card space-y-5 rounded-3xl border border-srapi-border bg-srapi-card p-6">
          <h4 className="font-serif text-lg italic text-srapi-text-primary">
            {t("activeChannels")}
          </h4>

          {loading ? (
            <div className="py-12 text-center">
              <Spinner size={24} label={t("queryRegistry")} />
            </div>
          ) : keys.length === 0 ? (
            <div className="space-y-3.5 rounded-2xl border border-dashed border-srapi-border py-16 text-center">
              <Key
                size={28}
                aria-hidden="true"
                className="mx-auto text-srapi-text-secondary opacity-40"
              />
              <p className="font-serif text-xs font-bold text-srapi-text-primary">{t("noKeys")}</p>
              <p className="font-mono text-xs text-srapi-text-secondary">{t("noKeysDesc")}</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("keyName")}</TableHead>
                  <TableHead>{t("prefix")}</TableHead>
                  <TableHead>{t("allowedModels")}</TableHead>
                  <TableHead>{t("status")}</TableHead>
                  <TableHead>{t("created")}</TableHead>
                  <TableHead className="text-right">{t("actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((k) => (
                  <TableRow key={k.id}>
                    <TableCell className="font-sans font-bold text-srapi-text-primary">
                      {k.name}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <code className="rounded border border-srapi-border bg-srapi-card-muted px-2 py-0.5 text-xs text-srapi-text-secondary">
                          {k.prefix}
                        </code>
                        <button
                          type="button"
                          onClick={() => handleCopy(k.prefix, k.id)}
                          aria-label={`Copy prefix for ${k.name}`}
                          className="rounded border border-transparent p-1 text-srapi-text-secondary transition-all hover:border-srapi-border hover:bg-srapi-card-muted hover:text-srapi-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary"
                        >
                          {copiedId === k.id ? (
                            <Check size={12} aria-hidden="true" className="text-green-700" />
                          ) : (
                            <Copy size={12} aria-hidden="true" />
                          )}
                        </button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex max-w-[260px] flex-wrap gap-1">
                        {k.allowed_models.map((m) => (
                          <Badge key={m} size="sm">
                            {m}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={k.status === "active" ? "success" : "neutral"}>
                        {k.status === "active" ? textActiveBadge : textDisabledBadge}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-srapi-text-secondary">
                      {new Date(k.created_at).toLocaleDateString()}{" "}
                      {new Date(k.created_at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant={k.status === "active" ? "danger" : "outline"}
                        onClick={() => handleToggleStatus(k.id, k.status)}
                        disabled={toggleMutation.isPending}
                      >
                        <Power size={11} aria-hidden="true" />
                        {k.status === "active" ? t("revoke") : t("activate")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>

        {/* Create dialog */}
        <Dialog
          open={showCreateModal}
          onOpenChange={(open) => {
            setShowCreateModal(open);
            if (!open) form.clearErrors();
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("deployTitle")}</DialogTitle>
              <DialogDescription>{t("deployDesc")}</DialogDescription>
            </DialogHeader>

            <form onSubmit={onSubmit} className="space-y-5">
              <div className="space-y-1.5">
                <Label htmlFor="api-key-name">{t("keyNickname")}</Label>
                <Input
                  id="api-key-name"
                  placeholder={placeholderName}
                  aria-invalid={!!form.formState.errors.name}
                  aria-describedby={
                    form.formState.errors.name ? "api-key-name-error" : undefined
                  }
                  {...form.register("name")}
                />
                {form.formState.errors.name ? (
                  <p
                    id="api-key-name-error"
                    role="alert"
                    className="text-xs text-srapi-error"
                  >
                    {form.formState.errors.name.message}
                  </p>
                ) : null}
              </div>

              <div className="space-y-2">
                <Label>{t("allowedTargetModels")}</Label>
                <div className="grid grid-cols-2 gap-2">
                  {DEFAULT_MODELS.map((m) => {
                    const isSelected = selectedModels.includes(m);
                    return (
                      <button
                        key={m}
                        type="button"
                        onClick={() => handleModelToggle(m)}
                        aria-pressed={isSelected}
                        className={cn(
                          "flex items-center justify-between rounded-lg border p-2.5 font-mono text-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary",
                          isSelected
                            ? "border-srapi-primary bg-srapi-primary/5 font-bold text-srapi-primary"
                            : "border-srapi-border bg-srapi-bg text-srapi-text-secondary hover:bg-srapi-card-muted",
                        )}
                      >
                        {m}
                        {isSelected ? <Sparkles size={10} aria-hidden="true" /> : null}
                      </button>
                    );
                  })}
                </div>
                {form.formState.errors.allowedModels ? (
                  <p role="alert" className="text-xs text-srapi-error">
                    {form.formState.errors.allowedModels.message}
                  </p>
                ) : null}
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="api-key-groups">{t("scopeGroupsCsv")}</Label>
                <Input
                  id="api-key-groups"
                  placeholder="group-01, group-02"
                  value={groupsCsv}
                  onChange={(e) => {
                    const next = e.target.value;
                    setGroupsCsv(next);
                    form.setValue("groupIds", parseGroupIdsCsv(next), {
                      shouldValidate: true,
                    });
                  }}
                />
                {form.formState.errors.groupIds ? (
                  <p role="alert" className="text-xs text-srapi-error">
                    {form.formState.errors.groupIds.message}
                  </p>
                ) : null}
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => setShowCreateModal(false)}
                >
                  {t("cancel")}
                </Button>
                <Button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? t("deploying") : t("deployChannel")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
}
