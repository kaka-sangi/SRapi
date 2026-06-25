"use client";

import { type ReactNode, useRef, useState } from "react";
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
import { LabelWithHelp } from "@/components/ui/help-tooltip";
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

/** Map an enum string array into select options with i18n labels via `common.<value>` keys. */
export function enumOptions(
  values: readonly string[],
  t?: (key: string) => string,
): { value: string; label: string }[] {
  return values.map((value) => ({
    value,
    label: t ? t(`common.${value}`) : value,
  }));
}

type FieldType =
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
  /** Contextual help shown in a tooltip icon next to the label */
  help?: string;
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
  /** text inputs: native datalist autocomplete suggestions (opt-in, no effect on other field types) */
  suggestions?: string[];
  /** tuck this field under a collapsed "Advanced" section to keep the common path short */
  advanced?: boolean;
  /** render the stored draft value as an input string (default: String(value)) */
  format?: (value: TDraft[keyof TDraft]) => string;
  /** convert the raw input string back to the stored value (default: identity string) */
  parse?: (raw: string) => unknown;
  /** require a non-empty value; an empty field blocks submit with an inline error */
  required?: boolean;
  /** custom per-field validation; return an error string to block submit, or undefined */
  validate?: (value: TDraft[keyof TDraft], draft: TDraft) => string | undefined;
}

/**
 * Generic create/edit dialog. Each admin resource supplies a flat `initial`
 * draft (from its `emptyXForm()` / `xFormFromEntity()` helper), a `fields`
 * config, and the matching `buildBody` transform.
 *
 * Validation happens in two layers: per-field `required`/`validate` rules
 * surface inline under each field BEFORE submit; `buildBody` and server errors
 * still surface in the shared role=alert footer.
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
  onDraftChange,
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
  onDraftChange?: (fieldName: string, value: unknown, draft: TDraft, setDraft: React.Dispatch<React.SetStateAction<TDraft>>) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  // Callers mount this dialog only while open (`{target ? <ResourceFormDialog…/> : null}`),
  // so the component mounts fresh per open and `initial` is read once — no reset effect needed.
  const [draft, setDraft] = useState<TDraft>(initial);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);

  const primaryFields = fields.filter((f) => !f.advanced);
  const advancedFields = fields.filter((f) => f.advanced);

  function setField(name: string, value: unknown) {
    setDraft((prev) => {
      const next = { ...prev, [name]: value } as TDraft;
      onDraftChange?.(name, value, next, setDraft);
      return next;
    });
    // Clear a field's error the moment the admin edits it — errors should feel
    // responsive, not stick around after the problem is fixed.
    setFieldErrors((prev) => {
      if (!prev[name]) return prev;
      const next = { ...prev };
      delete next[name];
      return next;
    });
  }

  /** Run `required` + per-field `validate` rules; populate inline errors. */
  function validateFields(): Record<string, string> {
    const errs: Record<string, string> = {};
    for (const field of fields) {
      const value = draft[field.name];
      if (field.required) {
        const empty =
          value == null ||
          (typeof value === "string" && value.trim() === "") ||
          (Array.isArray(value) && value.length === 0) ||
          (typeof value === "object" &&
            value !== null &&
            !Array.isArray(value) &&
            Object.keys(value).length === 0);
        if (empty) {
          errs[field.name] = t("adminCommon.required");
          continue;
        }
      }
      const message = field.validate?.(value, draft);
      if (message) errs[field.name] = message;
    }
    setFieldErrors(errs);
    // If a problem hides under the collapsed Advanced section, open it so the
    // admin can actually see the field that failed.
    if (advancedFields.some((f) => errs[f.name])) setAdvancedOpen(true);
    return errs;
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const errs = validateFields();
    if (Object.keys(errs).length > 0) {
      // Validation choreography: shake the form, then bring the first offending
      // field into view + focus it (deferred a tick so a just-opened Advanced
      // section is mounted before we look the field up).
      const formEl = formRef.current;
      if (formEl) {
        formEl.classList.remove("anim-shake");
        void formEl.offsetWidth; // reflow so the animation re-triggers each time
        formEl.classList.add("anim-shake");
      }
      const firstErr = fields.find((f) => errs[f.name])?.name;
      if (firstErr) {
        setTimeout(() => {
          const ctrl = document.getElementById(`field-${firstErr}`);
          ctrl?.scrollIntoView({ block: "nearest" });
          (ctrl as HTMLElement | null)?.focus?.({ preventScroll: true });
        }, 0);
      }
      return;
    }
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
        <form ref={formRef} onSubmit={onSubmit} noValidate>
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
          <div className="mt-4 min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pr-2">
            {primaryFields.map((field) => (
              <FieldRow
                key={field.name}
                field={field}
                value={draft[field.name]}
                error={fieldErrors[field.name]}
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
                <div
                  className={cn(
                    "grid overflow-hidden transition-[grid-template-rows] duration-300 ease-out",
                    advancedOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
                  )}
                  inert={!advancedOpen || undefined}
                >
                  <div className="min-h-0 overflow-hidden">
                    <div className="space-y-4 border-t border-srapi-border px-3.5 py-4">
                      {advancedFields.map((field) => (
                        <FieldRow
                          key={field.name}
                          field={field}
                          value={draft[field.name]}
                          error={fieldErrors[field.name]}
                          disabled={busy}
                          onChange={(value) => setField(field.name, value)}
                        />
                      ))}
                    </div>
                  </div>
                </div>
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

/** Inline field footer: shows error AND hint together so context isn't lost. */
function FieldMessage({ id, error, hint }: { id: string; error?: string; hint?: string }) {
  if (!error && !hint) return null;
  return (
    <div id={id} className="mt-1 space-y-0.5">
      {error && (
        <p role="alert" className="text-2xs text-srapi-error">
          {error}
        </p>
      )}
      {hint && (
        <p className={error ? "text-2xs text-srapi-text-tertiary/60" : "text-2xs text-srapi-text-tertiary"}>
          {hint}
        </p>
      )}
    </div>
  );
}

function FieldRow<TDraft extends object>({
  field,
  value,
  onChange,
  error,
  disabled,
}: {
  field: FieldConfig<TDraft>;
  value: TDraft[keyof TDraft];
  onChange: (value: unknown) => void;
  error?: string;
  disabled?: boolean;
}) {
  const { t } = useLanguage();
  const id = `field-${field.name}`;
  const msgId = `${id}-msg`;
  const type = field.type ?? "text";
  const asString = field.format ? field.format(value) : value == null ? "" : String(value);
  const parse = field.parse ?? ((raw: string) => raw);
  const invalid = error ? true : undefined;
  const describedBy = error || field.hint ? msgId : undefined;

  const requiredMark = field.required ? (
    <span className="ml-0.5 text-srapi-error" aria-hidden="true">*</span>
  ) : null;

  const renderFieldLabel = (children: ReactNode, htmlFor?: string, className?: string) => {
    if (field.help) {
      return (
        <LabelWithHelp htmlFor={htmlFor} help={field.help}>
          {children}
          {requiredMark}
        </LabelWithHelp>
      );
    }
    return (
      <Label htmlFor={htmlFor} className={className}>
        {children}
        {requiredMark}
      </Label>
    );
  };

  if (type === "switch") {
    return (
      <div className="flex items-center justify-between gap-4">
        <div>
          {renderFieldLabel(field.label, id)}
          <FieldMessage id={msgId} error={error} hint={field.hint} />
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
        {renderFieldLabel(field.label)}
        <div className="mt-1.5">
          <KeyValueEditor
            value={object}
            onChange={(next) => onChange(next)}
            disabled={disabled}
            addLabel={t("adminCommon.addField")}
          />
        </div>
        <FieldMessage id={msgId} error={error} hint={field.hint} />
      </div>
    );
  }

  if (type === "multiselect") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    const toggle = (key: string) =>
      onChange(selected.includes(key) ? selected.filter((k) => k !== key) : [...selected, key]);
    return (
      <div>
        {renderFieldLabel(field.label)}
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
        <FieldMessage id={msgId} error={error} hint={field.hint} />
      </div>
    );
  }

  if (type === "tags") {
    const tags = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        {renderFieldLabel(field.label, id)}
        <div className="mt-1.5">
          <TagInput
            id={id}
            value={tags}
            onChange={(next) => onChange(next)}
            placeholder={field.placeholder}
            disabled={disabled}
          />
        </div>
        <FieldMessage id={msgId} error={error} hint={field.hint} />
      </div>
    );
  }

  if (type === "combobox") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        {renderFieldLabel(field.label, id)}
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
        <FieldMessage id={msgId} error={error} hint={field.hint} />
      </div>
    );
  }

  if (type === "select") {
    return (
      <div>
        {renderFieldLabel(field.label, id)}
        <Select value={asString} onValueChange={(next) => onChange(next)} disabled={disabled}>
          <SelectTrigger id={id} aria-invalid={invalid} aria-describedby={describedBy}>
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
        <FieldMessage id={msgId} error={error} hint={field.hint} />
      </div>
    );
  }

  if (type === "textarea" || type === "json") {
    return (
      <div>
        {renderFieldLabel(field.label, id)}
        <Textarea
          id={id}
          placeholder={field.placeholder}
          spellCheck={type === "json" ? false : undefined}
          className={type === "json" ? "min-h-28 font-mono text-xs" : undefined}
          value={asString}
          disabled={disabled}
          aria-invalid={invalid}
          aria-describedby={describedBy}
          onChange={(event) => onChange(parse(event.target.value))}
        />
        <FieldMessage id={msgId} error={error} hint={field.hint} />
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

  const listId =
    field.suggestions && field.suggestions.length > 0 ? `${id}-suggest` : undefined;
  return (
    <div>
      {renderFieldLabel(field.label, id)}
      <Input
        id={id}
        type={inputType}
        list={listId}
        placeholder={field.placeholder}
        value={asString}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        onChange={(event) => onChange(parse(event.target.value))}
      />
      {listId ? (
        <datalist id={listId}>
          {field.suggestions!.map((s) => (
            <option key={s} value={s} />
          ))}
        </datalist>
      ) : null}
      <FieldMessage id={msgId} error={error} hint={field.hint} />
    </div>
  );
}
