import { describe, expect, it } from "vitest";
import { isImageFile, isTextFile } from "@/lib/image-utils";

// File mockShim — happy-dom's File is fine, but we need a tiny adapter that
// lets us inject (name, type) cheaply. The classifiers only read .name and
// .type so a plain object cast is enough.
function mockFile(name: string, type: string): File {
  return { name, type, size: 0 } as unknown as File;
}

// The copilot file-attach flow branches on these. isImageFile decides
// "render as inline thumbnail + send as image_part"; isTextFile decides
// "read as text + fold into the prompt". A misclassification here either
// blows the model context with binary garbage or refuses to attach a
// legitimate log file. Both bugs are quietly bad, so pin the contract.

describe("isImageFile", () => {
  it("returns true for any image/* MIME", () => {
    expect(isImageFile(mockFile("a.png", "image/png"))).toBe(true);
    expect(isImageFile(mockFile("a.jpg", "image/jpeg"))).toBe(true);
    expect(isImageFile(mockFile("a.webp", "image/webp"))).toBe(true);
  });

  it("returns false for non-image MIMEs even when the extension looks visual", () => {
    // The classifier MUST look at MIME, not at the filename. A renamed PDF
    // with a .png extension would otherwise sneak past.
    expect(isImageFile(mockFile("sneaky.png", "application/pdf"))).toBe(false);
    expect(isImageFile(mockFile("notes.txt", "text/plain"))).toBe(false);
  });

  it("returns false for missing / unknown MIME", () => {
    expect(isImageFile(mockFile("a.png", ""))).toBe(false);
  });
});

describe("isTextFile", () => {
  it("accepts every text/* MIME", () => {
    expect(isTextFile(mockFile("a.txt", "text/plain"))).toBe(true);
    expect(isTextFile(mockFile("a.html", "text/html"))).toBe(true);
    expect(isTextFile(mockFile("a.csv", "text/csv"))).toBe(true);
  });

  it("accepts the application/* MIMEs the regex whitelists", () => {
    // These are the structured-data formats users routinely upload as logs
    // / configs / dumps. Pin each one so a "tidy up the regex" PR has to
    // explicitly add or remove a format.
    for (const t of [
      "application/json",
      "application/x-ndjson",
      "application/xml",
      "application/x-yaml",
      "application/yaml",
      "application/x-sh",
      "application/x-shellscript",
      "application/sql",
      "application/toml",
      "application/javascript",
      "application/typescript",
      "application/x-httpd-php",
      "application/graphql",
    ]) {
      expect(isTextFile(mockFile(`a.dat`, t))).toBe(true);
    }
  });

  it("rejects application/* MIMEs that aren't on the whitelist", () => {
    // PDF, zip, octet-stream must NOT be folded into the prompt as text.
    for (const t of [
      "application/pdf",
      "application/zip",
      "application/octet-stream",
      "application/x-rar-compressed",
    ]) {
      expect(isTextFile(mockFile("a.bin", t))).toBe(false);
    }
  });

  it("falls back to the extension when the MIME is unknown / empty", () => {
    // Most upload widgets give the file an empty type when the OS doesn't
    // know it. The extension whitelist must cover the long tail.
    expect(isTextFile(mockFile("server.log", ""))).toBe(true);
    expect(isTextFile(mockFile("config.toml", ""))).toBe(true);
    expect(isTextFile(mockFile("script.ts", ""))).toBe(true);
    expect(isTextFile(mockFile("module.go", "application/octet-stream"))).toBe(true);
    expect(isTextFile(mockFile("page.svelte", ""))).toBe(true);
  });

  it("handles dot-prefixed and extension-less names that ARE text", () => {
    // ".gitignore" → ext "gitignore" (in the set). "Makefile" → no dot
    // → whole name treated as ext → "makefile" (in the set). These are
    // common config files and must NOT be rejected as binary.
    expect(isTextFile(mockFile(".gitignore", ""))).toBe(true);
    expect(isTextFile(mockFile("Makefile", ""))).toBe(true);
    expect(isTextFile(mockFile("Dockerfile", ""))).toBe(true);
  });

  it("rejects unknown extensions with no MIME hint", () => {
    expect(isTextFile(mockFile("payload.bin", ""))).toBe(false);
    expect(isTextFile(mockFile("photo.heic", ""))).toBe(false);
  });
});
