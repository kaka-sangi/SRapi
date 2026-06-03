"use client";

import { useState } from "react";
import { ChevronDown } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import { TagInput } from "@/components/ui/tag-input";
import { MultiSelect } from "@/components/ui/multi-select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { cn } from "@/lib/cn";
import { adminErrorMessage } from "@/lib/admin-api";

/** Map an enum string array (e.g. PROXY_TYPES) into select options. */
export function enumOptions(values: readonly string[]): { value: string; label: string }[] {
  return values.map((value) => ({ value, label: value }));
}

export type FieldType =
  | "text"
  | "password"
  | "textarea"
  | "json"
  | "number"
  | "select"
  | "multiselect"
  | "combobox"
  | "tags"
  | "keyvalue"
  | "switch"
  | "datetime";

export interface FieldConfig<TDraft> {
  name: keyof TDraft & string;
  label: string;
  type?: FieldType;
  placeholder?: string;
  hint?: string;
  /** options for select / multiselect / combobox fields */
  options?: { value: string; label: string }[];
  /** combobox/tags: let the admin commit a typed value not in `options` */
  allowCustom?: boolean;
  /** combobox: placeholder shown in the search box */
  searchPlaceholder?: string;
  /** combobox: message when no option matches the search */
  emptyText?: string;
  /** combobox: label for the "add typed value" row */
  addCustomLabel?: (query: string) => string;
  /** tuck this field under a collapsed "Advanced" section to keep the common path short */
  advanced?: boolean;
  /** render the stored draft value as an input string (default: String(value)) */
  format?: (value: TDraft[keyof TDraft]) => string;
  /** convert the raw input string back to the stored value (default: identity string) */
  parse?: (raw: string) => unknown;
}

/**
 * Generic create/edit dialog. Each admin resource supplies a flat `initial`
 * draft (from its `emptyXForm()` / `xFormFromEntity()` helper), a `fields`
 * config, and the matching `buildBody` transform. Validation errors thrown by
 * `buildBody` and server errors from `submit` both surface inline via role=alert.
 *
 * Structure mirrors the working api-key-create-dialog: local draft state,
 * mutateAsync on submit, reset on (re)open.
 */
export function ResourceFormDialog<TDraft extends object, TBody>({
  open,
  onOpenChange,
  title,
  description,
  fields,
  initial,
  buildBody,
  submit,
  submitLabel,
  successMessage,
  isPending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  fields: FieldConfig<TDraft>[];
  initial: TDraft;
  buildBody: (draft: TDraft) => TBody;
  submit: (body: TBody) => Promise<unknown>;
  submitLabel?: string;
  successMessage: string;
  isPending?: boolean;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  // Callers mount this dialog only while open (`{target ? <ResourceFormDialog…/> : null}`),
  // so the component mounts fresh per open and `initial` is read once — no reset effect needed.
  const [draft, setDraft] = useState<TDraft>(initial);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);

  const primaryFields = fields.filter((f) => !f.advanced);
  const advancedFields = fields.filter((f) => f.advanced);

  function setField(name: string, value: unknown) {
    setDraft((prev) => ({ ...prev, [name]: value }) as TDraft);
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    let body: TBody;
    try {
      body = buildBody(draft);
    } catch (err) {
      setError(adminErrorMessage(err));
      return;
    }
    setSubmitting(true);
    try {
      await submit(body);
      toast({ title: successMessage, tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  }

  const busy = submitting || Boolean(isPending);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={onSubmit}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {/* Always describe the dialog: a visible hint when given, else an
                sr-only fallback from the title so Radix has an aria description. */}
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : (
              <DialogDescription className="sr-only">{title}</DialogDescription>
            )}
          </DialogHeader>
          <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
            {primaryFields.map((field) => (
              <FieldRow
                key={field.name}
                field={field}
                value={draft[field.name]}
                disabled={busy}
                onChange={(value) => setField(field.name, value)}
              />
            ))}
            {advancedFields.length > 0 ? (
              <div className="rounded-lg border border-srapi-border">
                <button
                  type="button"
                  onClick={() => setAdvancedOpen((v) => !v)}
                  className="flex w-full items-center justify-between px-3.5 py-2.5 text-left"
                  aria-expanded={advancedOpen}
                >
                  <span className="text-sm text-srapi-text-secondary">
                    {t("adminCommon.advanced")}
                  </span>
                  <ChevronDown
                    className={cn(
                      "size-4 text-srapi-text-tertiary transition-transform",
                      advancedOpen && "rotate-180",
                    )}
                  />
                </button>
                {advancedOpen ? (
                  <div className="space-y-4 border-t border-srapi-border px-3.5 py-4">
                    {advancedFields.map((field) => (
                      <FieldRow
                        key={field.name}
                        field={field}
                        value={draft[field.name]}
                        disabled={busy}
                        onChange={(value) => setField(field.name, value)}
                      />
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-6">
            <Button
              type="button"
              variant="ghost"
              disabled={busy}
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={busy}>
              {submitLabel ?? t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function FieldRow<TDraft extends object>({
  field,
  value,
  onChange,
  disabled,
}: {
  field: FieldConfig<TDraft>;
  value: TDraft[keyof TDraft];
  onChange: (value: unknown) => void;
  disabled?: boolean;
}) {
  const { t } = useLanguage();
  const id = `field-${field.name}`;
  const type = field.type ?? "text";
  const asString = field.format ? field.format(value) : value == null ? "" : String(value);
  const parse = field.parse ?? ((raw: string) => raw);

  if (type === "switch") {
    return (
      <div className="flex items-center justify-between gap-4">
        <div>
          <Label htmlFor={id} className="mb-0">
            {field.label}
          </Label>
          {field.hint ? (
            <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p>
          ) : null}
        </div>
        <Switch
          id={id}
          checked={Boolean(value)}
          disabled={disabled}
          onCheckedChange={(checked) => onChange(checked)}
        />
      </div>
    );
  }

  if (type === "keyvalue") {
    const object =
      value && typeof value === "object" && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : {};
    return (
      <div>
        <Label>{field.label}</Label>
        <div className="mt-1.5">
          <KeyValueEditor
            value={object}
            onChange={(next) => onChange(next)}
            disabled={disabled}
            addLabel={t("adminCommon.addField")}
          />
        </div>
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  if (type === "multiselect") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    const toggle = (key: string) =>
      onChange(selected.includes(key) ? selected.filter((k) => k !== key) : [...selected, key]);
    return (
      <div>
        <Label>{field.label}</Label>
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {(field.options ?? []).map((opt) => {
            const on = selected.includes(opt.value);
            return (
              <button
                key={opt.value}
                type="button"
                disabled={disabled}
                aria-pressed={on}
                onClick={() => toggle(opt.value)}
                className={cn(
                  "rounded-full border px-2.5 py-1 text-2xs transition-colors disabled:opacity-50",
                  on
                    ? "border-srapi-invert bg-srapi-invert text-srapi-invert-fg"
                    : "border-srapi-border text-srapi-text-secondary hover:border-srapi-text-tertiary hover:text-srapi-text-primary",
                )}
              >
                {opt.label}
              </button>
            );
          })}
        </div>
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  if (type === "tags") {
    const tags = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        <Label htmlFor={id}>{field.label}</Label>
        <div className="mt-1.5">
          <TagInput
            id={id}
            value={tags}
            onChange={(next) => onChange(next)}
            placeholder={field.placeholder}
            disabled={disabled}
          />
        </div>
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  if (type === "combobox") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        <Label htmlFor={id}>{field.label}</Label>
        <div className="mt-1.5">
          <MultiSelect
            id={id}
            value={selected}
            onChange={(next) => onChange(next)}
            options={field.options ?? []}
            placeholder={field.placeholder}
            searchPlaceholder={field.searchPlaceholder}
            emptyText={field.emptyText}
            allowCustom={field.allowCustom}
            addCustomLabel={field.addCustomLabel}
            disabled={disabled}
          />
        </div>
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  if (type === "select") {
    return (
      <div>
        <Label htmlFor={id}>{field.label}</Label>
        <Select value={asString} onValueChange={(next) => onChange(next)} disabled={disabled}>
          <SelectTrigger id={id}>
            <SelectValue placeholder={field.placeholder} />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  if (type === "textarea" || type === "json") {
    return (
      <div>
        <Label htmlFor={id}>{field.label}</Label>
        <Textarea
          id={id}
          placeholder={field.placeholder}
          spellCheck={type === "json" ? false : undefined}
          className={type === "json" ? "min-h-28 font-mono text-xs" : undefined}
          value={asString}
          disabled={disabled}
          onChange={(event) => onChange(parse(event.target.value))}
        />
        {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
      </div>
    );
  }

  const inputType =
    type === "password"
      ? "password"
      : type === "number"
        ? "number"
        : type === "datetime"
          ? "datetime-local"
          : "text";

  return (
    <div>
      <Label htmlFor={id}>{field.label}</Label>
      <Input
        id={id}
        type={inputType}
        placeholder={field.placeholder}
        value={asString}
        disabled={disabled}
        onChange={(event) => onChange(parse(event.target.value))}
      />
      {field.hint ? <p className="mt-1 text-2xs text-srapi-text-tertiary">{field.hint}</p> : null}
    </div>
  );
}
