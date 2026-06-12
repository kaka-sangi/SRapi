"use client";

import { X, Paperclip, ArrowUp, Square, FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useLanguage } from "@/context/LanguageContext";
import { imagePartToDataUrl, type CopilotImagePart, type CopilotFilePart } from "@/lib/image-utils";
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
  files = [],
  removeFile,
  onAttach,
  placeholder,
  extraControls,
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
  files?: CopilotFilePart[];
  removeFile?: (idx: number) => void;
  onAttach: () => void;
  placeholder: string;
  /** Optional extra toolbar controls rendered after the attach button. */
  extraControls?: React.ReactNode;
}) {
  const { t } = useLanguage();
  const canSend = (input.trim().length > 0 || images.length > 0 || files.length > 0) && !running;
  return (
    <div className="rounded-2xl border border-srapi-border bg-srapi-card p-2 shadow-sm transition-[box-shadow,border-color] duration-200 focus-within:border-srapi-text-secondary focus-within:shadow-[0_10px_34px_-14px_rgba(28,26,23,0.22)]">
      {images.length || files.length ? (
        <div className="flex flex-wrap gap-2 px-1 pb-2 pt-1">
          {images.map((img, idx) => (
            <div key={`img-${idx}`} className="group relative size-14 overflow-hidden rounded-lg border border-srapi-border">
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
          {files.map((file, idx) => (
            <div
              key={`file-${idx}`}
              className="group relative flex h-14 max-w-52 items-center gap-2 rounded-lg border border-srapi-border bg-srapi-card-muted px-2.5 pr-6"
              title={file.name}
            >
              <FileText className="size-4 shrink-0 text-srapi-text-tertiary" />
              <span className="min-w-0 truncate text-2xs text-srapi-text-secondary">{file.name}</span>
              {removeFile ? (
                <button
                  type="button"
                  onClick={() => removeFile(idx)}
                  aria-label={t("chat.removeFile")}
                  className="absolute right-0.5 top-0.5 flex size-4 items-center justify-center rounded-full bg-srapi-invert/80 text-srapi-invert-fg opacity-0 transition-opacity group-hover:opacity-100"
                >
                  <X className="size-3" />
                </button>
              ) : null}
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
            // Match the send button's disabled state: never send empty input or
            // while a stream is already in flight.
            if (canSend) onSend();
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
          <Button variant="ghost" size="icon" className="size-8 shrink-0" onClick={onAttach} aria-label={t("chat.attach")}>
            <Paperclip className="size-4" />
          </Button>
          {extraControls}
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
