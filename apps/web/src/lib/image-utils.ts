export interface CopilotImagePart {
  mime_type: string;
  /** Base64-encoded bytes, without the `data:…;base64,` prefix. */
  data: string;
}

/** Reads an image File into the raw-base64 shape the copilot API expects. */
export function fileToImagePart(file: File): Promise<CopilotImagePart> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("Failed to read image"));
    reader.onload = () => {
      const result = String(reader.result ?? "");
      const comma = result.indexOf(",");
      resolve({
        mime_type: file.type || "image/png",
        data: comma >= 0 ? result.slice(comma + 1) : result,
      });
    };
    reader.readAsDataURL(file);
  });
}

/** Rebuilds a displayable data URL from a stored image part (for thumbnails). */
export function imagePartToDataUrl(part: CopilotImagePart): string {
  return `data:${part.mime_type};base64,${part.data}`;
}
