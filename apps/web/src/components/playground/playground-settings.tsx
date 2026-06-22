"use client";

import { SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { useLanguage } from "@/context/LanguageContext";

export interface PlaygroundParams {
  system: string;
  /** Raw input value; "" means "use the model default". */
  temperature: string;
  /** Raw input value; "" means "use the model default". */
  maxTokens: string;
}

export const DEFAULT_PLAYGROUND_PARAMS: PlaygroundParams = {
  system: "",
  temperature: "",
  maxTokens: "",
};

export function playgroundParamsDirty(params: PlaygroundParams): boolean {
  return (
    params.system.trim() !== "" || params.temperature.trim() !== "" || params.maxTokens.trim() !== ""
  );
}

/** Composer toolbar popover holding the conversation-level model parameters
 * (system prompt, temperature, max tokens). A dot on the trigger marks
 * non-default settings so they are never silently in effect. */
export function PlaygroundSettings({
  params,
  onChange,
}: {
  params: PlaygroundParams;
  onChange: (next: PlaygroundParams) => void;
}) {
  const { t } = useLanguage();
  const dirty = playgroundParamsDirty(params);

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="relative size-8 shrink-0"
          aria-label={t("playground.settings")}
        >
          <SlidersHorizontal className="size-4" />
          {dirty ? (
            <span className="absolute right-1 top-1 size-1.5 rounded-full bg-srapi-primary" aria-hidden />
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 max-h-[min(28rem,var(--radix-popover-content-available-height))] overflow-y-auto p-3">
        <div className="space-y-3">
          <div>
            <Label htmlFor="pg-system">{t("playground.systemPrompt")}</Label>
            <Textarea
              id="pg-system"
              value={params.system}
              onChange={(e) => onChange({ ...params, system: e.target.value })}
              placeholder={t("playground.systemPromptPlaceholder")}
              rows={4}
              className="max-h-40"
            />
            <p className="mt-1 text-[11px] text-srapi-text-tertiary">{t("playground.systemPromptHint")}</p>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <Label htmlFor="pg-temperature">{t("playground.temperature")}</Label>
              <Input
                id="pg-temperature"
                type="number"
                min={0}
                max={2}
                step={0.1}
                inputMode="decimal"
                value={params.temperature}
                onChange={(e) => onChange({ ...params, temperature: e.target.value })}
                placeholder={t("playground.paramDefault")}
              />
            </div>
            <div>
              <Label htmlFor="pg-max-tokens">{t("playground.maxTokens")}</Label>
              <Input
                id="pg-max-tokens"
                type="number"
                min={1}
                inputMode="numeric"
                value={params.maxTokens}
                onChange={(e) => onChange({ ...params, maxTokens: e.target.value })}
                placeholder={t("playground.paramDefault")}
              />
            </div>
          </div>
          {dirty ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="w-full"
              onClick={() => onChange(DEFAULT_PLAYGROUND_PARAMS)}
            >
              {t("playground.resetParams")}
            </Button>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}
