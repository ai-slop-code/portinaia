import { useEffect, useState, type FormEvent } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { CssBaseline, ThemeProvider } from "@mui/material";

import { theme } from "../theme/theme";
import type { components } from "../api/generated";
import { apiClient, setCSRFToken } from "../api/client";
import { LoginForm } from "../features/auth/LoginForm";
import { AppShell } from "./AppShell";
import { OverviewPage } from "../pages/OverviewPage";
import { AddFlowPage } from "../pages/AddFlowPage";
import { ComputeListPage } from "../pages/ComputeListPage";
import { ComputeDetailPage } from "../pages/ComputeDetailPage";
import { DomainsPage } from "../pages/DomainsPage";
import { ProvidersPage } from "../pages/ProvidersPage";
import { PlacesPage } from "../pages/PlacesPage";
import { ActivityPage } from "../pages/ActivityPage";

type Session = components["schemas"]["Session"];

async function readSession(): Promise<Session | null> {
  const { data, response } = await apiClient.GET("/auth/session", {
    headers: { "Cache-Control": "no-store" },
  });
  if (response.status === 401) {
    setCSRFToken(undefined);
    return null;
  }
  if (!response.ok) {
    throw new Error(`session bootstrap failed with ${response.status}`);
  }
  const session = data ?? null;
  setCSRFToken(session?.csrf_token);
  return session;
}

export function Workspace() {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    void readSession()
      .then((current) => {
        setSession(current);
        setLoading(false);
      })
      .catch(() => {
        setError(
          "Could not check the current session. Retry by reloading the page.",
        );
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    const expired = () => {
      setCSRFToken(undefined);
      setSession(null);
    };
    const changed = () => setSession((current) => current);
    window.addEventListener("portinaia:session-expired", expired);
    window.addEventListener("portinaia:session-changed", changed);
    return () => {
      window.removeEventListener("portinaia:session-expired", expired);
      window.removeEventListener("portinaia:session-changed", changed);
    };
  }, []);

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const values = new FormData(event.currentTarget);
    const { data, response } = await apiClient.POST("/auth/login", {
      body: {
        username: String(values.get("username") ?? ""),
        password: String(values.get("password") ?? ""),
      },
    });
    if (!response.ok || !data) {
      setError(
        response.status === 401
          ? "Username or password is incorrect."
          : "Unable to sign in.",
      );
      return;
    }
    setCSRFToken(data.csrf_token);
    setSession(data);
  }

  async function logout() {
    if (!session) return;
    await apiClient.POST("/auth/logout");
    window.dispatchEvent(new Event("portinaia:session-expired"));
  }

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      {session ? (
        <AppShell username={session.username} onSignOut={() => void logout()}>
          <Routes>
            <Route path="/" element={<OverviewPage />} />
            <Route path="/add" element={<AddFlowPage />} />
            <Route path="/compute" element={<ComputeListPage />} />
            <Route path="/compute/:id" element={<ComputeDetailPage />} />
            <Route path="/domains" element={<DomainsPage />} />
            <Route path="/domains/:id" element={<DomainsPage />} />
            <Route path="/providers" element={<ProvidersPage />} />
            <Route path="/providers/:id" element={<ProvidersPage />} />
            <Route path="/places" element={<PlacesPage />} />
            <Route path="/places/:id" element={<PlacesPage />} />
            <Route path="/activity" element={<ActivityPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AppShell>
      ) : (
        <LoginForm error={error} loading={loading} onSubmit={login} />
      )}
    </ThemeProvider>
  );
}
