export type GoogleDrivePickerDocument = {
  id?: string;
  name?: string;
  url?: string;
  mimeType?: string;
};

export type GoogleDrivePickerConfig = {
  accessToken: string;
  apiKey: string;
  appId: string;
};

type PickerCallbackData = {
  action?: string;
  docs?: GoogleDrivePickerDocument[];
};

type DocsView = {
  setMode: (mode: string) => DocsView;
  setMimeTypes: (mimeTypes: string) => DocsView;
  setSelectFolderEnabled: (enabled: boolean) => DocsView;
};

type PickerBuilder = {
  setOAuthToken: (token: string) => PickerBuilder;
  setDeveloperKey: (key: string) => PickerBuilder;
  setAppId: (appId: string) => PickerBuilder;
  setOrigin: (origin: string) => PickerBuilder;
  addView: (view: DocsView) => PickerBuilder;
  setCallback: (callback: (data: PickerCallbackData) => void) => PickerBuilder;
  build: () => { setVisible: (visible: boolean) => void };
};

type GooglePickerNamespace = {
  PickerBuilder: new () => PickerBuilder;
  DocsView: new (viewId: string) => DocsView;
  ViewId: { DOCS: string };
  DocsViewMode: { LIST: string };
};

declare global {
  interface Window {
    gapi?: { load: (module: string, callback: () => void) => void };
    google?: { picker?: GooglePickerNamespace };
  }
}

const googleAPIScript = "https://apis.google.com/js/api.js";
const supportedMimeTypes = [
  "application/vnd.google-apps.document",
  "text/plain",
  "text/markdown",
  "text/html",
  "application/pdf",
].join(",");

let pickerLoader: Promise<GooglePickerNamespace> | undefined;

export function googleDriveDocumentLocation(document: GoogleDrivePickerDocument) {
  if (document.url) return document.url;
  return document.id
    ? `https://drive.google.com/open?id=${encodeURIComponent(document.id)}`
    : "";
}

export async function openGoogleDrivePicker(config: GoogleDrivePickerConfig) {
  const picker = await loadGooglePicker();
  return new Promise<GoogleDrivePickerDocument | undefined>((resolve) => {
    const view = new picker.DocsView(picker.ViewId.DOCS)
      .setMode(picker.DocsViewMode.LIST)
      .setMimeTypes(supportedMimeTypes)
      .setSelectFolderEnabled(false);
    new picker.PickerBuilder()
      .setOAuthToken(config.accessToken)
      .setDeveloperKey(config.apiKey)
      .setAppId(config.appId)
      .setOrigin(window.location.origin)
      .addView(view)
      .setCallback((data) => {
        if (data.action === "picked") resolve(data.docs?.[0]);
        if (data.action === "cancel") resolve(undefined);
      })
      .build()
      .setVisible(true);
  });
}

function loadGooglePicker() {
  if (window.google?.picker) return Promise.resolve(window.google.picker);
  if (pickerLoader) return pickerLoader;

  pickerLoader = new Promise<GooglePickerNamespace>((resolve, reject) => {
    const loadModule = () => {
      if (!window.gapi) {
        reject(new Error("Google API did not load"));
        return;
      }
      window.gapi.load("picker", () => {
        if (window.google?.picker) resolve(window.google.picker);
        else reject(new Error("Google Drive Picker did not load"));
      });
    };

    if (window.gapi) {
      loadModule();
      return;
    }

    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${googleAPIScript}"]`,
    );
    if (existing) {
      existing.addEventListener("load", loadModule, { once: true });
      existing.addEventListener(
        "error",
        () => reject(new Error("Could not load Google API")),
        { once: true },
      );
      return;
    }

    const script = document.createElement("script");
    script.src = googleAPIScript;
    script.async = true;
    script.addEventListener("load", loadModule, { once: true });
    script.addEventListener(
      "error",
      () => reject(new Error("Could not load Google API")),
      { once: true },
    );
    document.head.appendChild(script);
  }).catch((error) => {
    pickerLoader = undefined;
    throw error;
  });

  return pickerLoader;
}
