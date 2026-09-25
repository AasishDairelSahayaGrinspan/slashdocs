import { useEffect, useState } from "react";
import { clearCommentDrafts } from "./comment-drafts";

const SIGNED_OUT_KEY = "aidocs.frontend_signed_out";
const AUTH_STATE_EVENT = "aidocs:auth-state";

export function isFrontendSignedOut() {
  try {
    return localStorage.getItem(SIGNED_OUT_KEY) === "true";
  } catch {
    return false;
  }
}

export function setFrontendSignedOut(signedOut: boolean) {
  try {
    if (signedOut) {
      localStorage.setItem(SIGNED_OUT_KEY, "true");
      // Unsent comments must not outlive the session on a shared browser.
      clearCommentDrafts();
    } else {
      localStorage.removeItem(SIGNED_OUT_KEY);
    }
  } catch {}
  window.dispatchEvent(new Event(AUTH_STATE_EVENT));
}

export function useFrontendSignedOut() {
  const [signedOut, setSignedOut] = useState(isFrontendSignedOut);

  useEffect(() => {
    const refresh = () => setSignedOut(isFrontendSignedOut());
    window.addEventListener(AUTH_STATE_EVENT, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(AUTH_STATE_EVENT, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, []);

  return signedOut;
}
