import type { FileRef } from "@/api/types.legacy";

// Where the browser loads an uploaded file. Files are named by their
// SHA-256, so the address never points at different bytes.
export function fileUrl(ref: FileRef | string) {
  return `/api/files/${typeof ref === "string" ? ref : ref.sha256}`;
}
