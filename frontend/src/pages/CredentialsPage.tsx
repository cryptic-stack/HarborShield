import { useEffect, useState } from "react";
import { api } from "../api/client";

type Credential = {
  accessKey: string;
  userId: string;
  role: string;
  description: string;
  lastUsedAt: string;
  createdAt: string;
};

type CreatedCredential = {
  accessKey: string;
  secretKey: string;
  role: string;
  description: string;
  createdAt: string;
};

const roles = ["readonly", "bucket-admin", "auditor", "admin", "superadmin"];

function formatTimestamp(value: string) {
  if (!value) {
    return "Never used";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

export function CredentialsPage() {
  const [items, setItems] = useState<Credential[]>([]);
  const [description, setDescription] = useState("");
  const [role, setRole] = useState("readonly");
  const [userId, setUserId] = useState("");
  const [created, setCreated] = useState<CreatedCredential | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function load() {
    const result = await api<{ items: Credential[] }>("/credentials");
    setItems(result.items ?? []);
  }

  async function createCredential() {
    setLoading(true);
    setError("");
    try {
      const result = await api<CreatedCredential>("/credentials", {
        method: "POST",
        body: JSON.stringify({
          userId: userId.trim(),
          role,
          description: description.trim(),
        }),
      });
      setCreated(result);
      setDescription("");
      setRole("readonly");
      setUserId("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Credential creation failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Credentials</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Create S3 service credentials separately from platform login. Secret keys are only shown once at creation time.
        </p>
      </div>

      {created ? (
        <div className="rounded-3xl border border-amber-200 bg-amber-50 p-5">
          <div className="text-xs uppercase tracking-[0.2em] text-amber-700">New Credential</div>
          <div className="mt-3 grid gap-3 md:grid-cols-2">
            <div className="rounded-2xl bg-white px-4 py-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Access Key</div>
              <div className="mt-2 break-all font-mono text-sm text-ink">{created.accessKey}</div>
            </div>
            <div className="rounded-2xl bg-white px-4 py-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Secret Key</div>
              <div className="mt-2 break-all font-mono text-sm text-ink">{created.secretKey}</div>
            </div>
          </div>
          <p className="mt-3 text-sm text-amber-900">This secret will not be shown again after you leave or refresh this page.</p>
        </div>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-[360px_1fr]">
        <section className="rounded-3xl border border-slate-200 bg-slate-50 p-5">
          <h3 className="text-lg font-semibold text-ink">Create Credential</h3>
          <div className="mt-4 space-y-4">
            <label className="block space-y-2 text-sm text-slate-700">
              <span>Description</span>
              <input
                className="w-full rounded-2xl border border-slate-200 px-4 py-3"
                placeholder="backup automation"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </label>
            <label className="block space-y-2 text-sm text-slate-700">
              <span>Role</span>
              <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={role} onChange={(event) => setRole(event.target.value)}>
                {roles.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </label>
            <label className="block space-y-2 text-sm text-slate-700">
              <span>User ID</span>
              <input
                className="w-full rounded-2xl border border-slate-200 px-4 py-3"
                placeholder="optional platform user id"
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
              />
            </label>
            {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}
            <button className="rounded-2xl bg-ink px-5 py-3 text-white disabled:opacity-60" disabled={loading} onClick={() => void createCredential()}>
              {loading ? "Creating..." : "Create Credential"}
            </button>
          </div>
        </section>

        <section className="rounded-3xl border border-slate-200 p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-lg font-semibold text-ink">Issued Credentials</h3>
              <p className="mt-1 text-sm text-slate-500">Track role, creator association, and last-used activity for S3 access keys.</p>
            </div>
            <div className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{items.length} total</div>
          </div>
          <div className="mt-4 space-y-3">
            {items.length === 0 ? (
              <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No S3 credentials issued yet.</div>
            ) : (
              items.map((item) => (
                <div key={item.accessKey} className="rounded-2xl border border-slate-200 px-4 py-4">
                  <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                    <div>
                      <div className="font-mono text-sm text-ink">{item.accessKey}</div>
                      <div className="mt-1 text-sm text-slate-500">{item.description || "No description"}</div>
                    </div>
                    <div className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium uppercase tracking-[0.2em] text-slate-600">{item.role}</div>
                  </div>
                  <div className="mt-3 grid gap-3 text-sm text-slate-500 md:grid-cols-3">
                    <div>
                      <div className="text-xs uppercase tracking-[0.2em] text-slate-400">User Link</div>
                      <div className="mt-1 text-slate-600">{item.userId || "Unassigned service account"}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Last Used</div>
                      <div className="mt-1 text-slate-600">{formatTimestamp(item.lastUsedAt)}</div>
                    </div>
                    <div>
                      <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Created</div>
                      <div className="mt-1 text-slate-600">{formatTimestamp(item.createdAt)}</div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
