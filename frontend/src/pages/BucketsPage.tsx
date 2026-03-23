import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";

type Bucket = {
  id: string;
  name: string;
  tenant: string;
  storageClass: string;
  replicaTarget: number;
  effectiveStorageClass: string;
  effectiveReplicaTarget: number;
  createdAt: string;
};

type StorageClassPolicy = {
  name: string;
  label: string;
  description: string;
  defaultReplicas: number;
};

type SettingsSnapshot = {
  storageDefaultClass: string;
  storageClassPolicies: StorageClassPolicy[];
};

export function BucketsPage() {
  const [items, setItems] = useState<Bucket[]>([]);
  const [name, setName] = useState("");
  const [storageClass, setStorageClass] = useState("inherit");
  const [replicaTarget, setReplicaTarget] = useState("0");
  const [settings, setSettings] = useState<SettingsSnapshot | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function load() {
    const [bucketResult, settingsResult] = await Promise.all([
      api<{ items: Bucket[] }>("/buckets"),
      api<SettingsSnapshot>("/settings"),
    ]);
    setItems(bucketResult.items ?? []);
    setSettings(settingsResult);
  }

  async function createBucket() {
    const trimmed = name.trim();
    if (!trimmed) {
      setError("Enter a bucket name before creating it.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api("/buckets", {
        method: "POST",
        body: JSON.stringify({
          name: trimmed,
          storageClass,
          replicaTarget: Number.parseInt(replicaTarget || "0", 10) || 0,
        }),
      });
      setName("");
      setStorageClass("inherit");
      setReplicaTarget("0");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Bucket creation failed");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 className="text-3xl font-semibold text-ink">Buckets</h2>
          <p className="mt-1 text-sm text-slate-600">Create and browse logical containers for object data.</p>
        </div>
        <div className="flex gap-2">
          <button className="rounded-2xl bg-moss px-4 py-3 text-white disabled:opacity-60" disabled={busy} onClick={() => void createBucket()}>
            {busy ? "Creating..." : "Create"}
          </button>
        </div>
      </div>
      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}
      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="grid gap-4 md:grid-cols-3">
          <label className="space-y-2 text-sm text-slate-700">
            <span>Bucket Name</span>
            <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="new bucket name" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="space-y-2 text-sm text-slate-700">
            <span>Storage Class</span>
            <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={storageClass} onChange={(e) => setStorageClass(e.target.value)}>
              <option value="inherit">Inherit cluster default</option>
              {(settings?.storageClassPolicies ?? []).map((policy) => (
                <option key={policy.name} value={policy.name}>
                  {policy.label}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-2 text-sm text-slate-700">
            <span>Replica Target Override</span>
            <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={replicaTarget} onChange={(e) => setReplicaTarget(e.target.value)} placeholder="0" />
          </label>
        </div>
        <div className="mt-4 rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
          {storageClass === "inherit"
            ? `This bucket will inherit the cluster default storage class: ${formatStorageClass(settings?.storageDefaultClass ?? "standard")}.`
            : selectedPolicy(settings?.storageClassPolicies ?? [], storageClass)?.description ?? "This class defines the bucket's default durability behavior."}
        </div>
      </section>
      <div className="grid gap-3">
        {items.map((item) => (
          <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="font-medium text-ink">{item.name}</div>
                <div className="mt-1 text-sm text-slate-500">{item.tenant}</div>
                <div className="mt-2 text-xs text-slate-500">
                  Class: {formatStorageClass(item.effectiveStorageClass || item.storageClass || "inherit")} | Replica target: {item.effectiveReplicaTarget || item.replicaTarget || 0}
                </div>
              </div>
              <Link className="rounded-2xl bg-slate-100 px-4 py-2 text-sm text-slate-700" to={`/buckets/${item.id}`}>
                View Detail
              </Link>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function selectedPolicy(policies: StorageClassPolicy[], name: string) {
  return policies.find((policy) => policy.name === name);
}

function formatStorageClass(value: string) {
  switch (value) {
    case "reduced-redundancy":
      return "Reduced redundancy";
    case "archive-ready":
      return "Archive ready";
    case "inherit":
      return "Inherit cluster default";
    default:
      return "Standard";
  }
}
