import { useEffect, useState } from "react";
import { api, setAccessToken, setUnauthorizedHandler } from "../api/client";

type Session = {
  accessToken: string;
  refreshToken?: string;
  mustChangePassword?: boolean;
  user: {
    id: string;
    email: string;
    role: string;
  };
};

export type DeploymentSetupStatus = {
  completed: boolean;
  required: boolean;
  desiredStorageBackend: string;
  distributedScope: string;
  remoteEndpoints: string[];
  runtimeStorageBackend: string;
  runtimeEndpointCount: number;
  applyRequired: boolean;
  recommendedLocalProfile: string;
};

type DeploymentSetupInput = {
  mode: "single-node" | "distributed";
  distributedMode?: "local" | "remote";
  remoteEndpoints?: string[];
};

export function useSession() {
  const [session, setSession] = useState<Session | null>(null);
  const [setupStatus, setSetupStatus] = useState<DeploymentSetupStatus | null>(null);
  const [setupLoading, setSetupLoading] = useState(false);

  function clearSession() {
    setAccessToken("");
    window.localStorage.removeItem("harborshield-session");
    setSession(null);
    setSetupStatus(null);
  }

  async function loadSetupStatus() {
    setSetupLoading(true);
    try {
      const result = await api<DeploymentSetupStatus>("/setup/status");
      setSetupStatus(result);
    } finally {
      setSetupLoading(false);
    }
  }

  useEffect(() => {
    const stored = window.localStorage.getItem("harborshield-session");
    if (!stored) {
      return;
    }

    try {
      const parsed = JSON.parse(stored) as Session;
      if (!parsed?.accessToken || !parsed?.user?.id || !parsed?.user?.email || !parsed?.user?.role) {
        throw new Error("invalid session");
      }
      setAccessToken(parsed.accessToken);
      setSession(parsed);
    } catch {
      clearSession();
    }
  }, []);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      clearSession();
    });
    return () => setUnauthorizedHandler(null);
  }, []);

  useEffect(() => {
    if (!session) {
      return;
    }
    void loadSetupStatus().catch(() => {
      setSetupStatus(null);
      setSetupLoading(false);
    });
  }, [session?.accessToken]);

  async function login(email: string, password: string) {
    const result = await api<Session>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    setAccessToken(result.accessToken);
    window.localStorage.setItem("harborshield-session", JSON.stringify(result));
    setSession(result);
    await loadSetupStatus();
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    await api("/auth/change-password", {
      method: "POST",
      body: JSON.stringify({ currentPassword, newPassword }),
    });
    setSession((current) => {
      if (!current) {
        return current;
      }
      const nextSession = {
        ...current,
        mustChangePassword: false,
      };
      window.localStorage.setItem("harborshield-session", JSON.stringify(nextSession));
      return nextSession;
    });
    await loadSetupStatus();
  }

  async function logout() {
    const refreshToken = session?.refreshToken;
    try {
      if (refreshToken) {
        await api("/auth/logout", {
          method: "POST",
          body: JSON.stringify({ refreshToken }),
        });
      }
    } catch {
      // Always clear the local session, even if the server token is already expired or revoked.
    } finally {
      clearSession();
    }
  }

  async function completeSetup(input: DeploymentSetupInput) {
    const result = await api<DeploymentSetupStatus>("/setup/complete", {
      method: "POST",
      body: JSON.stringify(input),
    });
    setSetupStatus(result);
  }

  return { session, login, logout, changePassword, setupStatus, setupLoading, completeSetup };
}
