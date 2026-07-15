import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom";
import { QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster, toast } from "sonner";
import { TooltipProvider } from "@radix-ui/react-tooltip";
import "./styles.css";
import { APIError } from "@/api";
import { errorMessage } from "@/lib/errors";
import { ThemeProvider } from "@/lib/theme";
import { AppShell } from "@/components/layout/app-shell";
import { LandingPage } from "@/pages/landing";
import { LoginPage } from "@/pages/login";
import { DocumentsPage } from "@/pages/documents";
import { DocumentPage } from "@/pages/document";
import { FolderPage } from "@/pages/folder";
import { ServiceAccountsPage } from "@/pages/service-accounts";
import { DevelopersPage } from "@/pages/developers";
import { ProfilePage } from "@/pages/profile";
import { StartPage } from "@/pages/start";
import { RepositoriesPage } from "@/pages/repositories";
import { RepositoryPage } from "@/pages/repository";
import { PullRequestPage } from "@/pages/pull-request";
import { SettingsLayout } from "@/pages/settings-layout";
import { AccountSettingsPage } from "@/pages/settings-account";
import { AppearanceSettingsPage } from "@/pages/settings-appearance";
import { ConnectedAccountsSettingsPage } from "@/pages/settings-connections";
import { PublicPortalPage } from "@/pages/public-portal";
import { ReviewInboxPage } from "@/pages/review-inbox";
import { TemplatesPage } from "@/pages/templates";

// Query failures (other than auth, handled by route guards) and any mutation
// without its own onError surface as a toast.
const qc = new QueryClient({
  defaultOptions: {
    queries: { refetchOnWindowFocus: false },
    mutations: {
      onError: (e) => toast.error(errorMessage(e)),
    },
  },
  queryCache: new QueryCache({
    onError: (e, query) => {
      if (query.meta?.suppressGlobalError) return;
      if (e instanceof APIError && e.status === 401) return;
      toast.error(errorMessage(e));
    },
  }),
});

function App() {
  return (
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <TooltipProvider delayDuration={150}>
          <BrowserRouter>
            <Routes>
              <Route path="/" element={<LandingPage />} />
              <Route path="/login" element={<LoginPage />} />
              <Route path="/app" element={<AppShell />}>
                <Route index element={<Navigate to="documents" replace />} />
                <Route path="start" element={<StartPage />} />
                <Route path="documents" element={<DocumentsPage />} />
                <Route path="repositories" element={<RepositoriesPage />} />
                <Route path="review-inbox" element={<ReviewInboxPage />} />
                <Route path="templates" element={<TemplatesPage />} />
                <Route path="repos/:id" element={<RepositoryPage />} />
                <Route
                  path="repos/:id/settings"
                  element={<RepositoryTabRedirect tab="settings" />}
                />
                <Route path="repos/:id/pulls/:number" element={<PullRequestPage />} />
                <Route path="d/:id" element={<DocumentPage />} />
                <Route path="folders/:id" element={<FolderPage />} />
                <Route path="profile" element={<ProfilePage />} />
                <Route path="users/:id" element={<ProfilePage />} />
                <Route path="settings" element={<SettingsLayout />}>
                  <Route index element={<Navigate to="appearance" replace />} />
                  <Route path="account" element={<AccountSettingsPage />} />
                  <Route
                    path="connections"
                    element={<ConnectedAccountsSettingsPage />}
                  />
                  <Route path="appearance" element={<AppearanceSettingsPage />} />
                  <Route path="service-accounts" element={<ServiceAccountsPage />} />
                  <Route path="developer" element={<DevelopersPage />} />
                  <Route
                    path="profile"
                    element={<Navigate to="/app/profile" replace />}
                  />
                </Route>
                <Route
                  path="developers"
                  element={<Navigate to="/app/settings/developer" replace />}
                />
              </Route>
              <Route path="/portal/:slug" element={<PublicPortalPage />} />
              <Route path="/p/:slug" element={<PublicPortalPage />} />
              {/* legacy redirects */}
              <Route path="/documents/:id" element={<LegacyDocRedirect />} />
              <Route
                path="/service-accounts"
                element={<Navigate to="/app/settings/service-accounts" replace />}
              />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
            <Toaster position="bottom-right" theme="system" richColors />
          </BrowserRouter>
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  );
}

function LegacyDocRedirect() {
  const { id = "" } = useParams();
  return <Navigate to={`/app/d/${id}`} replace />;
}

function RepositoryTabRedirect({ tab }: { tab: string }) {
  const { id = "" } = useParams();
  return <Navigate to={`/app/repos/${id}?tab=${encodeURIComponent(tab)}`} replace />;
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
