import { FormEvent, useEffect, useState } from "react";

type Props = {
  onLogin: (email: string, password: string) => Promise<void>;
};

type OIDCStatus = {
  enabled: boolean;
  configured: boolean;
  loginReady: boolean;
  issuerUrl: string;
  clientId: string;
  redirectUrl: string;
  scopes: string[];
  roleClaim: string;
  defaultRole: string;
  roleMappings: string[];
  statusMessage: string;
  nextStepMessage: string;
};

function normalizeOIDCStatus(value: Partial<OIDCStatus> | null | undefined): OIDCStatus {
  return {
    enabled: Boolean(value?.enabled),
    configured: Boolean(value?.configured),
    loginReady: Boolean(value?.loginReady),
    issuerUrl: value?.issuerUrl ?? "",
    clientId: value?.clientId ?? "",
    redirectUrl: value?.redirectUrl ?? "",
    scopes: Array.isArray(value?.scopes) ? value!.scopes : [],
    roleClaim: value?.roleClaim ?? "",
    defaultRole: value?.defaultRole ?? "admin",
    roleMappings: Array.isArray(value?.roleMappings) ? value!.roleMappings : [],
    statusMessage: value?.statusMessage ?? "OIDC status is unavailable",
    nextStepMessage: value?.nextStepMessage ?? "Unable to load provider configuration details.",
  };
}

export function LoginPage({ onLogin }: Props) {
  const [email, setEmail] = useState("admin@example.com");
  const [password, setPassword] = useState("change_me_now");
  const [error, setError] = useState("");
  const [oidc, setOIDC] = useState<OIDCStatus | null>(null);

  useEffect(() => {
    void fetch("/api/v1/auth/oidc")
      .then(async (response) => {
        if (!response.ok) {
          throw new Error("Unable to load OIDC status");
        }
        return normalizeOIDCStatus((await response.json()) as Partial<OIDCStatus>);
      })
      .then(setOIDC)
      .catch(() => setOIDC(null));
  }, []);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    try {
      setError("");
      await onLogin(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    }
  }

  function startOIDCLogin() {
    window.location.href = "/api/v1/auth/oidc/start";
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-5xl grid gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
        <form
          onSubmit={handleSubmit}
          className="auth-surface rounded-[2rem] border border-slate-200 p-8 shadow-2xl"
        >
          <div className="text-xs uppercase tracking-[0.35em] text-ember">HarborShield</div>
          <h1 className="mt-3 text-3xl font-semibold text-ink">Admin Login</h1>
          <p className="mt-2 text-sm text-slate-600">Bootstrap with email and password first. OIDC status and role-mapping details are visible alongside the local sign-in flow.</p>
          <label className="auth-label mt-6 block text-sm">
            Email
            <input
              className="auth-input mt-2 w-full rounded-2xl px-4 py-3 text-slate-900 placeholder:text-slate-500"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
              spellCheck={false}
            />
          </label>
          <label className="auth-label mt-4 block text-sm">
            Password
            <input
              className="auth-input mt-2 w-full rounded-2xl px-4 py-3 text-slate-900 placeholder:text-slate-500"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
          </label>
          {error ? <div className="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
          <button className="mt-6 w-full rounded-2xl bg-ink px-4 py-3 text-sm font-medium text-white">Sign In</button>
        </form>

        <section
          className="auth-surface rounded-[2rem] border border-slate-200 p-6 shadow-2xl"
        >
          <div className="text-xs uppercase tracking-[0.3em] text-slate-500">OIDC Status</div>
          {oidc ? (
            <div className="mt-4 space-y-4">
              <div className="rounded-2xl bg-slate-50 p-4">
                <div className="text-sm font-medium text-ink">{oidc.statusMessage}</div>
                <p className="mt-2 text-sm text-slate-600">{oidc.nextStepMessage}</p>
              </div>
              <StatusRow label="Enabled" value={oidc.enabled ? "yes" : "no"} />
              <StatusRow label="Configured" value={oidc.configured ? "yes" : "no"} />
              <StatusRow label="Login Ready" value={oidc.loginReady ? "yes" : "no"} />
              <StatusRow label="Issuer" value={oidc.issuerUrl || "not configured"} />
              <StatusRow label="Client ID" value={oidc.clientId || "not configured"} />
              <StatusRow label="Redirect" value={oidc.redirectUrl || "not configured"} />
              <StatusRow label="Scopes" value={oidc.scopes.length ? oidc.scopes.join(", ") : "default"} />
              <StatusRow label="Role Claim" value={oidc.roleClaim || "not configured"} />
              <StatusRow label="Default Role" value={oidc.defaultRole || "admin"} />
              <StatusRow label="Role Mapping" value={oidc.roleMappings.length ? oidc.roleMappings.join(", ") : "not configured"} />
              <button
                className="mt-2 w-full rounded-2xl bg-slate-100 px-4 py-3 text-sm text-slate-700 disabled:opacity-60"
                disabled={!oidc.loginReady}
                onClick={startOIDCLogin}
                type="button"
              >
                {oidc.loginReady ? "Sign In With OIDC" : "OIDC Login Not Ready"}
              </button>
            </div>
          ) : (
            <div className="mt-4 rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">OIDC status is unavailable right now.</div>
          )}
        </section>
      </div>
    </div>
  );
}

function StatusRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-slate-200 px-4 py-3">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-1 break-words text-sm text-ink">{value}</div>
    </div>
  );
}
