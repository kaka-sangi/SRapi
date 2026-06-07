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

/** A text-extractable file attachment (logs, configs, JSON, CSV, code, …). The
 * copilot folds its content into the message text on send, so it works with any
 * model — no vision/document API support required. */
export interface CopilotFilePart {
  name: string;
  /** UTF-8 text content, truncated to MAX_TEXT_FILE_CHARS. */
  content: string;
  truncated: boolean;
}

// Hard ceilings: skip files too large to read into memory, and cap the text we
// fold into the prompt so a stray multi-MB log can't blow the context window.
const MAX_TEXT_FILE_BYTES = 8 * 1024 * 1024;
const MAX_TEXT_FILE_CHARS = 200_000;

const TEXT_FILE_EXTENSIONS = new Set([
  "txt", "text", "md", "markdown", "rst", "csv", "tsv", "json", "jsonl", "ndjson",
  "yaml", "yml", "xml", "html", "htm", "svg", "log", "ini", "conf", "cfg", "toml",
  "env", "properties", "sql", "sh", "bash", "zsh", "fish", "ps1", "bat",
  "js", "mjs", "cjs", "jsx", "ts", "tsx", "go", "py", "rb", "rs", "c", "h", "cc",
  "cpp", "hpp", "cs", "java", "kt", "kts", "php", "swift", "scala", "lua", "r",
  "css", "scss", "less", "vue", "svelte", "graphql", "proto", "diff", "patch",
  "tf", "hcl", "tfvars", "dockerfile", "makefile", "gitignore", "editorconfig",
  "lock", "gradle", "cmake",
]);

export function isImageFile(file: File): boolean {
  return file.type.startsWith("image/");
}

/** True when the file is reasonably read as UTF-8 text (by MIME or extension). */
export function isTextFile(file: File): boolean {
  if (file.type.startsWith("text/")) return true;
  if (
    /^application\/(json|x-ndjson|xml|x-yaml|yaml|x-sh|x-shellscript|sql|toml|javascript|typescript|x-httpd-php|graphql)\b/.test(
      file.type,
    )
  ) {
    return true;
  }
  const name = file.name.toLowerCase();
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : name;
  return TEXT_FILE_EXTENSIONS.has(ext);
}

/** Reads a text file into the shape the copilot folds into its prompt. Rejects
 * binary/oversized files so we never paste garbage at the model. */
export function fileToTextPart(file: File): Promise<CopilotFilePart> {
  return new Promise((resolve, reject) => {
    if (file.size > MAX_TEXT_FILE_BYTES) {
      reject(new Error("file too large"));
      return;
    }
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("Failed to read file"));
    reader.onload = () => {
      const raw = String(reader.result ?? "");
      const truncated = raw.length > MAX_TEXT_FILE_CHARS;
      resolve({
        name: file.name,
        content: truncated ? raw.slice(0, MAX_TEXT_FILE_CHARS) : raw,
        truncated,
      });
    };
    reader.readAsText(file);
  });
}
