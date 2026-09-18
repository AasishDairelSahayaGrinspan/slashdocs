import { useEffect, useState } from "react";
import { api } from "@/api";

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
    } else {
      localStorage.removeItem(SIGNED_OUT_KEY);
    }
  } catch {}
  window.dispatchEvent(new Event(AUTH_STATE_EVENT));
}

// signOut ends the session on the server, which revokes the session token and
// clears its cookie. The local flag is only set once the server confirms, so a
// failed sign-out never leaves a still-valid session looking signed out; the
// rejection is left for the caller to report.
export async function signOut() {
  await api.logout();
  setFrontendSignedOut(true);
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
