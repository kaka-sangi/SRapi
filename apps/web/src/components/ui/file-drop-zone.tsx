"use client";

import { useCallback, useRef, useState } from "react";
import { Upload, X } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";

interface FileDropZoneProps {
  accept?: string;
  multiple?: boolean;
  disabled?: boolean;
  hint?: string;
  onFiles: (files: File[]) => void;
  fileNames?: string[];
  onClearFiles?: () => void;
  className?: string;
}

export function FileDropZone({
  accept = ".json",
  multiple = false,
  disabled = false,
  hint,
  onFiles,
  fileNames,
  onClearFiles,
  className,
}: FileDropZoneProps) {
  const { t } = useLanguage();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const handleFiles = useCallback(
    (fileList: FileList | null) => {
      if (!fileList || fileList.length === 0) return;
      onFiles(Array.from(fileList));
    },
    [onFiles],
  );

  const onDragOver = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      if (!disabled) setDragging(true);
    },
    [disabled],
  );

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragging(false);
      if (!disabled) handleFiles(e.dataTransfer.files);
    },
    [disabled, handleFiles],
  );

  const hasFiles = fileNames && fileNames.length > 0;

  return (
    <div className={className}>
      <div
        role="button"
        aria-label={hint ?? t("common.uploadFiles")}
        tabIndex={disabled ? -1 : 0}
        onClick={() => !disabled && inputRef.current?.click()}
        onKeyDown={(e) => {
          if (!disabled && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault();
            inputRef.current?.click();
          }
        }}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        className={cn(
          "flex cursor-pointer flex-col items-center gap-2 rounded-2xl border-2 border-dashed p-8 text-center transition-colors",
          dragging
            ? "border-srapi-primary bg-srapi-accent-soft"
            : "border-srapi-border bg-srapi-card-muted/30 hover:border-srapi-border-strong hover:bg-srapi-card-muted/50",
          disabled && "pointer-events-none opacity-50",
        )}
      >
        <span className="grid size-9 place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary [&>svg]:size-4">
          <Upload />
        </span>
        {hint ? (
          <p className="text-xs text-srapi-text-tertiary">{hint}</p>
        ) : null}
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          multiple={multiple}
          className="hidden"
          disabled={disabled}
          onChange={(e) => {
            handleFiles(e.target.files);
            e.target.value = "";
          }}
        />
      </div>

      {hasFiles ? (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {fileNames.map((name) => (
            <span
              key={name}
              className="inline-flex items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary"
            >
              {name}
            </span>
          ))}
          {onClearFiles ? (
            <button
              type="button"
              onClick={onClearFiles}
              aria-label={t("common.removeFile")}
              className="inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[11px] text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-secondary"
            >
              <X className="size-3" />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
