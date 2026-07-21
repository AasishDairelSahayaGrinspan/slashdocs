// The multipart upload the server expects (POST /v1/documents/:id/versions)
// takes a file part, so edited HTML is wrapped in a File. The filename is
// cosmetic — the server stores the bytes and derives everything else — but it
// must end in .html so the upload reads correctly in logs and error messages.
export const EDITED_HTML_FILENAME = "document.html";

export function htmlToFile(html: string): File {
  return new File([html], EDITED_HTML_FILENAME, { type: "text/html" });
}
