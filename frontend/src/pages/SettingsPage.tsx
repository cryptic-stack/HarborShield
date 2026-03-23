import { useEffect, useState } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

type SettingsSnapshot = {
  appEnv: string;
  region: string;
  defaultTenant: string;
  presignTTL: string;
  maxUploadSizeBytes: number;
  storageBackend: string;
  storageRoot: string;
  storageDistributedEndpoints: string[];
  storageDistributedReplicas: number;
  storageDefaultClass: string;
  storageSupportedClasses: string[];
  storageClassPolicies: { name: string; label: string; description: string; defaultReplicas: number }[];
  storageEncrypted: boolean;
  clamavEnabled: boolean;
  malwareScanMode: string;
  adminIpAllowlist: string[];
  corsOrigins: string[];
  logLevel: string;
  oidcEnabled: boolean;
  oidcIssuerUrl: string;
  oidcClientId: string;
  oidcClientSecretConfigured: boolean;
  oidcRedirectUrl: string;
  oidcScopes: string[];
  oidcRoleClaim: string;
  oidcDefaultRole: string;
  oidcRoleMap: Record<string, string>;
};

function normalizeSettingsSnapshot(value: Partial<SettingsSnapshot> | null | undefined): SettingsSnapshot {
  return {
    appEnv: value?.appEnv ?? "unknown",
    region: value?.region ?? "not configured",
    defaultTenant: value?.defaultTenant ?? "default",
    presignTTL: value?.presignTTL ?? "not configured",
    maxUploadSizeBytes: typeof value?.maxUploadSizeBytes === "number" ? value.maxUploadSizeBytes : 0,
    storageBackend: value?.storageBackend ?? "local",
    storageRoot: value?.storageRoot ?? "not configured",
    storageDistributedEndpoints: Array.isArray(value?.storageDistributedEndpoints) ? value.storageDistributedEndpoints : [],
    storageDistributedReplicas: typeof value?.storageDistributedReplicas === "number" ? value.storageDistributedReplicas : 0,
    storageDefaultClass: value?.storageDefaultClass ?? "standard",
    storageSupportedClasses: Array.isArray(value?.storageSupportedClasses) ? value.storageSupportedClasses : [],
    storageClassPolicies: Array.isArray(value?.storageClassPolicies) ? value.storageClassPolicies : [],
    storageEncrypted: Boolean(value?.storageEncrypted),
    clamavEnabled: Boolean(value?.clamavEnabled),
    malwareScanMode: value?.malwareScanMode ?? "advisory",
    adminIpAllowlist: Array.isArray(value?.adminIpAllowlist) ? value.adminIpAllowlist : [],
    corsOrigins: Array.isArray(value?.corsOrigins) ? value.corsOrigins : [],
    logLevel: value?.logLevel ?? "info",
    oidcEnabled: Boolean(value?.oidcEnabled),
    oidcIssuerUrl: value?.oidcIssuerUrl ?? "",
    oidcClientId: value?.oidcClientId ?? "",
    oidcClientSecretConfigured: Boolean(value?.oidcClientSecretConfigured),
    oidcRedirectUrl: value?.oidcRedirectUrl ?? "",
    oidcScopes: Array.isArray(value?.oidcScopes) ? value.oidcScopes : [],
    oidcRoleClaim: value?.oidcRoleClaim ?? "",
    oidcDefaultRole: value?.oidcDefaultRole ?? "admin",
    oidcRoleMap: value?.oidcRoleMap && typeof value.oidcRoleMap === "object" ? value.oidcRoleMap : {},
  };
}

export function SettingsPage() {
  const [snapshot, setSnapshot] = useState<SettingsSnapshot | null>(null);
  const [error, setError] = useState("");
  const [oidcNotice, setOidcNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [oidcBusy, setOidcBusy] = useState(false);
  const [oidcTestBusy, setOidcTestBusy] = useState(false);
  const [defaultStorageClass, setDefaultStorageClass] = useState("standard");
  const [standardReplicas, setStandardReplicas] = useState("1");
  const [reducedReplicas, setReducedReplicas] = useState("1");
  const [archiveReplicas, setArchiveReplicas] = useState("1");
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [oidcIssuerUrl, setOidcIssuerUrl] = useState("");
  const [oidcClientId, setOidcClientId] = useState("");
  const [oidcClientSecret, setOidcClientSecret] = useState("");
  const [oidcRedirectUrl, setOidcRedirectUrl] = useState("");
  const [oidcScopes, setOidcScopes] = useState("openid, email, profile");
  const [oidcRoleClaim, setOidcRoleClaim] = useState("");
  const [oidcDefaultRole, setOidcDefaultRole] = useState("admin");
  const [oidcRoleMapText, setOidcRoleMapText] = useState("");

  useEffect(() => {
    void api<SettingsSnapshot>("/settings")
      .then((result) => {
        const normalized = normalizeSettingsSnapshot(result);
        setSnapshot(normalized);
        hydrateStoragePolicy(normalized, setDefaultStorageClass, setStandardReplicas, setReducedReplicas, setArchiveReplicas);
        hydrateOIDCSettings(normalized, setOidcEnabled, setOidcIssuerUrl, setOidcClientId, setOidcClientSecret, setOidcRedirectUrl, setOidcScopes, setOidcRoleClaim, setOidcDefaultRole, setOidcRoleMapText);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load settings"));
  }, []);

  async function saveStoragePolicy() {
    setBusy(true);
    setError("");
    try {
      await api("/settings/storage-policy", {
        method: "PATCH",
        body: JSON.stringify({
          defaultStorageClass,
          standardReplicas: Number.parseInt(standardReplicas || "0", 10) || 0,
          reducedRedundancyReplicas: Number.parseInt(reducedReplicas || "0", 10) || 0,
          archiveReadyReplicas: Number.parseInt(archiveReplicas || "0", 10) || 0,
        }),
      });
      const refreshed = normalizeSettingsSnapshot(await api<SettingsSnapshot>("/settings"));
      setSnapshot(refreshed);
      hydrateStoragePolicy(refreshed, setDefaultStorageClass, setStandardReplicas, setReducedReplicas, setArchiveReplicas);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save storage policy");
    } finally {
      setBusy(false);
    }
  }

  async function saveOIDCSettings() {
    setOidcBusy(true);
    setError("");
    setOidcNotice("");
    try {
      await api("/settings/oidc", {
        method: "PATCH",
        body: JSON.stringify({
          enabled: oidcEnabled,
          issuerUrl: oidcIssuerUrl.trim(),
          clientId: oidcClientId.trim(),
          clientSecret: oidcClientSecret,
          redirectUrl: oidcRedirectUrl.trim(),
          scopes: parseCSVList(oidcScopes),
          roleClaim: oidcRoleClaim.trim(),
          defaultRole: oidcDefaultRole,
          roleMap: parseRoleMapText(oidcRoleMapText),
        }),
      });
      const refreshed = normalizeSettingsSnapshot(await api<SettingsSnapshot>("/settings"));
      setSnapshot(refreshed);
      hydrateOIDCSettings(refreshed, setOidcEnabled, setOidcIssuerUrl, setOidcClientId, setOidcClientSecret, setOidcRedirectUrl, setOidcScopes, setOidcRoleClaim, setOidcDefaultRole, setOidcRoleMapText);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save OIDC settings");
    } finally {
      setOidcBusy(false);
    }
  }

  async function testOIDCConnection() {
    setOidcTestBusy(true);
    setError("");
    setOidcNotice("");
    try {
      const result = await api<{ issuerUrl: string; authorizationEndpoint: string; tokenEndpoint: string; jwksUrl: string; message: string }>("/settings/oidc/test", {
        method: "POST",
      });
      setOidcNotice(`${result.message}. Authorization endpoint: ${result.authorizationEndpoint || "not advertised"}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to test OIDC settings");
    } finally {
      setOidcTestBusy(false);
    }
  }

  async function clearOIDCSecret() {
    setOidcBusy(true);
    setError("");
    setOidcNotice("");
    try {
      await api("/settings/oidc/clear-secret", { method: "POST" });
      const refreshed = normalizeSettingsSnapshot(await api<SettingsSnapshot>("/settings"));
      setSnapshot(refreshed);
      hydrateOIDCSettings(refreshed, setOidcEnabled, setOidcIssuerUrl, setOidcClientId, setOidcClientSecret, setOidcRedirectUrl, setOidcScopes, setOidcRoleClaim, setOidcDefaultRole, setOidcRoleMapText);
      setOidcNotice("Stored OIDC client secret cleared.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to clear OIDC client secret");
    } finally {
      setOidcBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Settings</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Review the live runtime posture for this deployment, including security defaults, upload limits, and identity settings.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      {snapshot ? (
        <div className="space-y-6">
          <section className="rounded-3xl border border-slate-200 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-lg font-semibold text-ink">Cluster Storage Policy</h3>
                <p className="mt-1 text-sm text-slate-500">Manage the default storage class and live replica behavior for distributed buckets from the admin plane.</p>
              </div>
              <button className="rounded-2xl bg-ink px-4 py-3 text-sm text-white disabled:opacity-60" disabled={busy} onClick={() => void saveStoragePolicy()} type="button">
                {busy ? "Saving..." : "Save Storage Policy"}
              </button>
            </div>
            <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <label className="space-y-2 text-sm text-slate-700">
                <span>Default Storage Class</span>
                <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={defaultStorageClass} onChange={(event) => setDefaultStorageClass(event.target.value)}>
                  {snapshot.storageClassPolicies.map((policy) => (
                    <option key={policy.name} value={policy.name}>
                      {policy.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Standard Replicas</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={standardReplicas} onChange={(event) => setStandardReplicas(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Reduced Redundancy Replicas</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={reducedReplicas} onChange={(event) => setReducedReplicas(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Archive Ready Replicas</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={archiveReplicas} onChange={(event) => setArchiveReplicas(event.target.value)} />
              </label>
            </div>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              {snapshot.storageClassPolicies.map((policy) => (
                <div key={policy.name} className="rounded-2xl bg-slate-50 p-4">
                  <div className="text-sm font-medium text-ink">{policy.label}</div>
                  <p className="mt-2 text-sm text-slate-600">{policy.description}</p>
                </div>
              ))}
            </div>
          </section>

          <section className="rounded-3xl border border-slate-200 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-lg font-semibold text-ink">OIDC Provider Settings</h3>
                <p className="mt-1 text-sm text-slate-500">Configure the issuer, client credentials, scopes, and role mapping for provider-backed admin login.</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <button className="rounded-2xl border border-slate-300 px-4 py-3 text-sm text-slate-700 disabled:opacity-60" disabled={oidcTestBusy || oidcBusy} onClick={() => void testOIDCConnection()} type="button">
                  {oidcTestBusy ? "Testing..." : "Test Connection"}
                </button>
                <button
                  className="rounded-2xl border border-slate-300 px-4 py-3 text-sm text-slate-700 disabled:opacity-60"
                  disabled={oidcBusy || !snapshot.oidcClientSecretConfigured}
                  onClick={() => void clearOIDCSecret()}
                  type="button"
                >
                  {oidcBusy ? "Working..." : "Clear Stored Secret"}
                </button>
                <button className="rounded-2xl bg-ink px-4 py-3 text-sm text-white disabled:opacity-60" disabled={oidcBusy || oidcTestBusy} onClick={() => void saveOIDCSettings()} type="button">
                  {oidcBusy ? "Saving..." : "Save OIDC Settings"}
                </button>
              </div>
            </div>
            <div className="mt-4 grid gap-4 md:grid-cols-2">
              <label className="space-y-2 text-sm text-slate-700">
                <span>OIDC Enabled</span>
                <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcEnabled ? "enabled" : "disabled"} onChange={(event) => setOidcEnabled(event.target.value === "enabled")}>
                  <option value="disabled">Disabled</option>
                  <option value="enabled">Enabled</option>
                </select>
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Default Role</span>
                <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcDefaultRole} onChange={(event) => setOidcDefaultRole(event.target.value)}>
                  <option value="superadmin">Superadmin</option>
                  <option value="admin">Admin</option>
                  <option value="auditor">Auditor</option>
                  <option value="bucket-admin">Bucket admin</option>
                  <option value="readonly">Readonly</option>
                </select>
              </label>
              <label className="space-y-2 text-sm text-slate-700 md:col-span-2">
                <span>Issuer URL</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcIssuerUrl} onChange={(event) => setOidcIssuerUrl(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Client ID</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcClientId} onChange={(event) => setOidcClientId(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Client Secret</span>
                <input
                  className="w-full rounded-2xl border border-slate-200 px-4 py-3"
                  placeholder={snapshot.oidcClientSecretConfigured ? "Stored. Enter a new secret to replace it." : "Enter client secret"}
                  type="password"
                  value={oidcClientSecret}
                  onChange={(event) => setOidcClientSecret(event.target.value)}
                />
              </label>
              <label className="space-y-2 text-sm text-slate-700 md:col-span-2">
                <span>Redirect URL</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcRedirectUrl} onChange={(event) => setOidcRedirectUrl(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Scopes</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcScopes} onChange={(event) => setOidcScopes(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700">
                <span>Role Claim</span>
                <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={oidcRoleClaim} onChange={(event) => setOidcRoleClaim(event.target.value)} />
              </label>
              <label className="space-y-2 text-sm text-slate-700 md:col-span-2">
                <span>Role Mapping</span>
                <textarea
                  className="min-h-32 w-full rounded-2xl border border-slate-200 px-4 py-3"
                  placeholder={"group-a=admin\ngroup-b=readonly"}
                  value={oidcRoleMapText}
                  onChange={(event) => setOidcRoleMapText(event.target.value)}
                />
              </label>
            </div>
            <div className="mt-4 rounded-2xl bg-slate-50 p-4 text-sm text-slate-600">
              The client secret is never returned to the UI after it is saved. Leave it blank to keep the currently stored secret.
            </div>
            {oidcNotice ? <div className="mt-4 rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{oidcNotice}</div> : null}
          </section>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <SettingCard label="Environment" value={snapshot.appEnv} />
            <SettingCard label="S3 Region" value={snapshot.region} />
            <SettingCard label="Default Tenant" value={snapshot.defaultTenant} />
            <SettingCard label="Presign TTL" value={snapshot.presignTTL} />
            <SettingCard label="Upload Limit" value={formatBytes(snapshot.maxUploadSizeBytes)} />
            <SettingCard label="Log Level" value={snapshot.logLevel} />
            <SettingCard label="Storage Backend" value={snapshot.storageBackend} />
            <SettingCard label="Storage Root" value={snapshot.storageRoot} />
            <SettingCard label="Storage Nodes" value={snapshot.storageDistributedEndpoints.length ? snapshot.storageDistributedEndpoints.join(", ") : "Local-only deployment"} />
            <SettingCard label="Replica Target" value={snapshot.storageBackend === "distributed" ? String(snapshot.storageDistributedReplicas || snapshot.storageDistributedEndpoints.length) : "Local-only deployment"} />
            <SettingCard label="Default Storage Class" value={formatStorageClass(snapshot.storageDefaultClass)} />
            <SettingCard
              label="Supported Storage Classes"
              value={snapshot.storageSupportedClasses.length ? snapshot.storageSupportedClasses.map(formatStorageClass).join(", ") : "Standard"}
            />
            <SettingCard
              label="Storage Class Profiles"
              value={
                snapshot.storageClassPolicies.length
                  ? snapshot.storageClassPolicies.map((policy) => `${policy.label}: ${policy.defaultReplicas} replicas`).join(", ")
                  : "Standard: cluster default replicas"
              }
            />
            <SettingCard label="Storage Encryption" value={snapshot.storageEncrypted ? "Enabled by default" : "Disabled"} />
            <SettingCard label="Malware Scanning" value={snapshot.clamavEnabled ? "Enabled" : "Disabled"} />
            <SettingCard label="Malware Scan Mode" value={snapshot.malwareScanMode} />
            <SettingCard label="Authentication" value={snapshot.oidcEnabled ? "Local sign-in and OIDC configured" : "Local email and password only"} />
            <SettingCard label="OIDC Status" value={snapshot.oidcEnabled ? "Configured" : "Disabled"} />
            <SettingCard label="OIDC Issuer" value={snapshot.oidcIssuerUrl || "not configured"} />
            <SettingCard label="OIDC Client ID" value={snapshot.oidcClientId || "not configured"} />
            <SettingCard label="OIDC Client Secret" value={snapshot.oidcClientSecretConfigured ? "Stored securely" : "Not configured"} />
            <SettingCard label="OIDC Redirect" value={snapshot.oidcRedirectUrl || "not configured"} />
            <SettingCard label="OIDC Scopes" value={snapshot.oidcScopes.length ? snapshot.oidcScopes.join(", ") : "Default scopes"} />
            <SettingCard label="OIDC Role Claim" value={snapshot.oidcRoleClaim || "Not configured"} />
            <SettingCard label="OIDC Default Role" value={snapshot.oidcDefaultRole || "admin"} />
            <SettingCard
              label="OIDC Role Mapping"
              value={Object.keys(snapshot.oidcRoleMap).length ? Object.entries(snapshot.oidcRoleMap).map(([key, role]) => `${key} -> ${role}`).join(", ") : "Not configured"}
            />
            <SettingCard label="Admin IP Allowlist" value={snapshot.adminIpAllowlist.length ? snapshot.adminIpAllowlist.join(", ") : "Not configured"} />
            <SettingCard label="CORS Origins" value={snapshot.corsOrigins.length ? snapshot.corsOrigins.join(", ") : "Not configured"} />
          </div>
        </div>
      ) : (
        <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Loading runtime settings...</div>
      )}
    </div>
  );
}

function SettingCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-3xl border border-slate-200 p-5">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-2 break-words text-base font-medium text-ink">{value}</div>
    </div>
  );
}

function formatStorageClass(value: string) {
  switch (value) {
    case "reduced-redundancy":
      return "Reduced redundancy";
    case "archive-ready":
      return "Archive ready";
    default:
      return "Standard";
  }
}

function hydrateStoragePolicy(
  snapshot: SettingsSnapshot,
  setDefaultStorageClass: (value: string) => void,
  setStandardReplicas: (value: string) => void,
  setReducedReplicas: (value: string) => void,
  setArchiveReplicas: (value: string) => void,
) {
  setDefaultStorageClass(snapshot.storageDefaultClass);
  setStandardReplicas(String(findReplica(snapshot.storageClassPolicies, "standard")));
  setReducedReplicas(String(findReplica(snapshot.storageClassPolicies, "reduced-redundancy")));
  setArchiveReplicas(String(findReplica(snapshot.storageClassPolicies, "archive-ready")));
}

function hydrateOIDCSettings(
  snapshot: SettingsSnapshot,
  setEnabled: (value: boolean) => void,
  setIssuerUrl: (value: string) => void,
  setClientId: (value: string) => void,
  setClientSecret: (value: string) => void,
  setRedirectUrl: (value: string) => void,
  setScopes: (value: string) => void,
  setRoleClaim: (value: string) => void,
  setDefaultRole: (value: string) => void,
  setRoleMapText: (value: string) => void,
) {
  setEnabled(snapshot.oidcEnabled);
  setIssuerUrl(snapshot.oidcIssuerUrl);
  setClientId(snapshot.oidcClientId);
  setClientSecret("");
  setRedirectUrl(snapshot.oidcRedirectUrl);
  setScopes(snapshot.oidcScopes.join(", "));
  setRoleClaim(snapshot.oidcRoleClaim);
  setDefaultRole(snapshot.oidcDefaultRole || "admin");
  setRoleMapText(
    Object.keys(snapshot.oidcRoleMap)
      .sort()
      .map((key) => `${key}=${snapshot.oidcRoleMap[key]}`)
      .join("\n"),
  );
}

function parseCSVList(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseRoleMapText(value: string) {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .reduce<Record<string, string>>((result, line) => {
      const parts = line.split("=");
      if (parts.length < 2) {
        return result;
      }
      const key = parts.shift()?.trim() ?? "";
      const role = parts.join("=").trim();
      if (key && role) {
        result[key] = role;
      }
      return result;
    }, {});
}

function findReplica(policies: SettingsSnapshot["storageClassPolicies"], name: string) {
  return policies.find((policy) => policy.name === name)?.defaultReplicas ?? 1;
}
