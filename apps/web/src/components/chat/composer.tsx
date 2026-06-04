"use client";

import { X, Paperclip, ArrowUp, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useLanguage } from "@/context/LanguageContext";
import { imagePartToDataUrl, type CopilotImagePart } from "@/lib/image-utils";
import { ModelPicker } from "./model-picker";
import { EffortPicker } from "./effort-picker";
import type { ReasoningEffort } from "./types";

/** The chat composer: textarea + model/effort pickers + image attach + send/stop.
 * Shared by the admin copilot and the user playground. */
export function Composer({
  input,
  setInput,
  onSend,
  onStop,
  running,
  models,
  model,
  setModel,
  effort,
  setEffort,
  images,
  removeImage,
  onAttach,
  placeholder,
}: {
  input: string;
  setInput: (v: string) => void;
  onSend: () => void;
  onStop: () => void;
  running: boolean;
  models: string[];
  model: string;
  setModel: (v: string) => void;
  effort: ReasoningEffort;
  setEffort: (v: ReasoningEffort) => void;
  images: CopilotImagePart[];
  removeImage: (idx: number) => void;
  onAttach: () => void;
  placeholder: string;
}) {
  const { t } = useLanguage();
  const canSend = (input.trim().length > 0 || images.length > 0) && !running;
  return (
    <div className="rounded-2xl border border-srapi-border bg-srapi-card p-2 shadow-sm transition-shadow focus-within:border-srapi-text-tertiary focus-within:shadow-md">
      {images.length ? (
        <div className="flex flex-wrap gap-2 px-1 pb-2 pt-1">
          {images.map((img, idx) => (
            <div key={idx} className="group relative size-14 overflow-hidden rounded-lg border border-srapi-border">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={imagePartToDataUrl(img)} alt="" className="size-full object-cover" />
              <button
                type="button"
                onClick={() => removeImage(idx)}
                aria-label={t("chat.removeImage")}
                className="absolute right-0.5 top-0.5 flex size-4 items-center justify-center rounded-full bg-srapi-invert/80 text-srapi-invert-fg opacity-0 transition-opacity group-hover:opacity-100"
              >
                <X className="size-3" />
              </button>
            </div>
          ))}
        </div>
      ) : null}

      <Textarea
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            onSend();
          }
        }}
        placeholder={placeholder}
        className="max-h-48 min-h-[3rem] w-full resize-none border-0 bg-transparent px-2 py-1.5 shadow-none focus-visible:ring-0"
        rows={1}
      />

      <div className="flex items-center justify-between gap-2 px-1 pt-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <ModelPicker value={model} models={models} onChange={setModel} />
          <EffortPicker value={effort} onChange={setEffort} />
          <Button variant="ghost" size="icon" className="size-8 shrink-0" onClick={onAttach} aria-label={t("chat.attachImage")}>
            <Paperclip className="size-4" />
          </Button>
        </div>
        {running ? (
          <Button variant="outline" size="icon" className="size-8 rounded-full" onClick={onStop} aria-label={t("chat.stop")}>
            <Square className="size-3.5" />
          </Button>
        ) : (
          <Button variant="primary" size="icon" className="size-8 rounded-full" onClick={onSend} disabled={!canSend} aria-label={t("chat.send")}>
            <ArrowUp className="size-4" />
          </Button>
        )}
      </div>
    </div>
  );
}
