// frontend/src/lib/drive.ts
export class DriveAccessError extends Error {
  status: number;
  constructor(status: number) {
    super(status === 403 ? "no-drive-access" : status === 404 ? "drive-file-missing" : `drive-error-${status}`);
    this.status = status;
  }
}

const DRIVE_SCOPE = "https://www.googleapis.com/auth/drive.readonly";

export function getDriveAccessToken(clientId: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const g = (window as any).google;
    if (!g?.accounts?.oauth2) {
      reject(new Error("GIS not loaded"));
      return;
    }
    const client = g.accounts.oauth2.initTokenClient({
      client_id: clientId,
      scope: DRIVE_SCOPE,
      callback: (resp: { access_token?: string; error?: string }) => {
        if (resp.access_token) resolve(resp.access_token);
        else reject(new Error(resp.error || "token-failed"));
      },
      // Fires when the consent popup is dismissed or fails to open; without it
      // the promise (and the Drive render query) would hang forever.
      error_callback: (err: { type?: string }) =>
        reject(new Error(err?.type || "token-popup-failed")),
    });
    client.requestAccessToken();
  });
}

export async function fetchDriveFile(fileId: string, token: string, resourceKey?: string): Promise<string> {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (resourceKey) headers["X-Goog-Drive-Resource-Keys"] = `${fileId}/${resourceKey}`;
  const res = await fetch(
    `https://www.googleapis.com/drive/v3/files/${encodeURIComponent(fileId)}?alt=media`,
    { headers },
  );
  if (!res.ok) throw new DriveAccessError(res.status);
  return res.text();
}
