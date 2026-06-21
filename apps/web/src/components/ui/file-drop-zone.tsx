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
          "flex cursor-pointer flex-col items-center gap-1.5 rounded-xl border-2 border-dashed px-4 py-5 text-center transition-colors",
          dragging
            ? "border-srapi-text-tertiary bg-srapi-bg-sunken"
            : "border-srapi-border bg-srapi-card-muted hover:border-srapi-text-tertiary",
          disabled && "pointer-events-none opacity-50",
        )}
      >
        <Upload className="size-5 text-srapi-text-tertiary" />
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
        <div className="mt-2 flex flex-wrap gap-1.5">
          {fileNames.map((name) => (
            <span
              key={name}
              className="inline-flex items-center gap-1 rounded-md bg-srapi-card-muted px-2 py-0.5 text-2xs text-srapi-text-secondary"
            >
              {name}
            </span>
          ))}
          {onClearFiles ? (
            <button
              type="button"
              onClick={onClearFiles}
              aria-label={t("common.removeFile")}
              className="inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
            >
              <X className="size-3" />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
