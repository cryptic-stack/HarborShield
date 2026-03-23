import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

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
type ObjectItem = { id: string; key: string; versionId: string; sizeBytes: number; etag: string; createdAt: string; tags: Record<string, string>; legalHold: boolean };
type VersionItem = { id: string; versionId: string; sizeBytes: number; etag: string; createdAt: string; isDeleteMarker: boolean; tags: Record<string, string>; legalHold: boolean };

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function BucketDetailPage() {
  const { bucketId = "" } = useParams();
  const [bucket, setBucket] = useState<Bucket | null>(null);
  const [items, setItems] = useState<ObjectItem[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [versions, setVersions] = useState<VersionItem[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [tagDraft, setTagDraft] = useState("");
  const [copyKey, setCopyKey] = useState("");
  const [storageClass, setStorageClass] = useState("inherit");
  const [replicaTarget, setReplicaTarget] = useState("0");

  useEffect(() => {
    if (!bucketId) {
      return;
    }
    void Promise.all([
      api<{ items: Bucket[] }>("/buckets"),
      api<{ items: ObjectItem[] }>(`/buckets/${bucketId}/objects`),
    ])
      .then(([bucketResult, objectResult]) => {
        const bucketItems = bucketResult.items ?? [];
        const objectItems = objectResult.items ?? [];
        const matchedBucket = bucketItems.find((item) => item.id === bucketId) ?? null;
        setBucket(matchedBucket);
        setStorageClass(matchedBucket?.storageClass ?? "inherit");
        setReplicaTarget(String(matchedBucket?.replicaTarget ?? 0));
        setItems(objectItems);
        if (objectItems[0]) {
          setSelectedKey(objectItems[0].key);
          setCopyKey(`${objectItems[0].key}.copy`);
        }
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load bucket detail"));
  }, [bucketId]);

  useEffect(() => {
    if (!bucketId || !selectedKey) {
      setVersions([]);
      return;
    }
    setCopyKey(`${selectedKey}.copy`);
    void api<{ items: VersionItem[] }>(`/buckets/${bucketId}/objects/versions?key=${encodeURIComponent(selectedKey)}`)
      .then((result) => {
        const versionItems = result.items ?? [];
        setVersions(versionItems);
        setTagDraft(formatTags(versionItems[0]?.tags ?? {}));
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load object versions"));
  }, [bucketId, selectedKey]);

  async function refreshData(nextSelectedKey = selectedKey) {
    if (!bucketId) {
      return;
    }
    const [bucketResult, objectResult] = await Promise.all([
      api<{ items: Bucket[] }>("/buckets"),
      api<{ items: ObjectItem[] }>(`/buckets/${bucketId}/objects`),
    ]);
    const bucketItems = bucketResult.items ?? [];
    const objectItems = objectResult.items ?? [];
    const matchedBucket = bucketItems.find((item) => item.id === bucketId) ?? null;
    setBucket(matchedBucket);
    setStorageClass(matchedBucket?.storageClass ?? "inherit");
    setReplicaTarget(String(matchedBucket?.replicaTarget ?? 0));
    setItems(objectItems);
    if (nextSelectedKey) {
      const versionsResult = await api<{ items: VersionItem[] }>(`/buckets/${bucketId}/objects/versions?key=${encodeURIComponent(nextSelectedKey)}`);
      const versionItems = versionsResult.items ?? [];
      setVersions(versionItems);
      setTagDraft(formatTags(versionItems[0]?.tags ?? {}));
    }
  }

  async function saveDurability() {
    if (!bucketId) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/durability`, {
        method: "PATCH",
        body: JSON.stringify({
          storageClass,
          replicaTarget: Number.parseInt(replicaTarget || "0", 10) || 0,
        }),
      });
      await refreshData(selectedKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save bucket durability");
    } finally {
      setBusy(false);
    }
  }

  async function restoreSelected() {
    if (!bucketId || !selectedKey) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/objects/restore`, {
        method: "POST",
        body: JSON.stringify({ key: selectedKey }),
      });
      await refreshData(selectedKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to restore object");
    } finally {
      setBusy(false);
    }
  }

  async function saveTags() {
    if (!bucketId || !selectedKey) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/objects/tags`, {
        method: "PUT",
        body: JSON.stringify({ key: selectedKey, tags: parseTags(tagDraft) }),
      });
      await refreshData(selectedKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save object tags");
    } finally {
      setBusy(false);
    }
  }

  async function copySelected() {
    if (!bucketId || !selectedKey || !copyKey) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/objects/copy`, {
        method: "POST",
        body: JSON.stringify({
          sourceBucketId: bucketId,
          sourceKey: selectedKey,
          destinationKey: copyKey,
          replaceTags: true,
          tags: parseTags(tagDraft),
        }),
      });
      await refreshData(copyKey);
      setSelectedKey(copyKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to copy object");
    } finally {
      setBusy(false);
    }
  }

  async function toggleLegalHold(versionId: string, legalHold: boolean) {
    if (!bucketId || !selectedKey) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/objects/legal-hold`, {
        method: "PUT",
        body: JSON.stringify({ key: selectedKey, versionId, legalHold }),
      });
      await refreshData(selectedKey);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update legal hold");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Bucket Detail</div>
          <h2 className="mt-1 text-3xl font-semibold text-ink">{bucket?.name ?? "Unknown bucket"}</h2>
          <p className="mt-1 text-sm text-slate-600">
            Review bucket identity, tenant placement, and the current object inventory from the admin plane.
          </p>
        </div>
        <Link className="rounded-2xl bg-slate-100 px-4 py-3 text-sm text-slate-700" to="/uploads">
          Open Upload Manager
        </Link>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      {bucket ? (
        <div className="grid gap-4 md:grid-cols-3">
          <SummaryCard label="Bucket ID" value={bucket.id} />
          <SummaryCard label="Tenant" value={bucket.tenant} />
          <SummaryCard label="Bucket Storage Class" value={formatStorageClass(bucket.storageClass || "inherit")} />
          <SummaryCard label="Bucket Replica Override" value={bucket.replicaTarget > 0 ? String(bucket.replicaTarget) : "Inherited"} />
          <SummaryCard label="Effective Storage Class" value={formatStorageClass(bucket.effectiveStorageClass || "standard")} />
          <SummaryCard label="Effective Replica Target" value={String(bucket.effectiveReplicaTarget || 0)} />
          <SummaryCard label="Created" value={formatTimestamp(bucket.createdAt)} />
        </div>
      ) : null}

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">Bucket Durability</h3>
            <p className="mt-1 text-sm text-slate-500">Choose a named durability class or inherit the cluster default, then optionally override the replica target for this bucket.</p>
          </div>
          <button className="rounded-2xl bg-ink px-4 py-3 text-sm text-white disabled:opacity-60" type="button" onClick={() => void saveDurability()} disabled={busy}>
            {busy ? "Saving..." : "Save Durability"}
          </button>
        </div>
        <div className="mt-4 grid gap-4 md:grid-cols-2">
          <label className="space-y-2 text-sm text-slate-700">
            <span>Storage Class</span>
            <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={storageClass} onChange={(event) => setStorageClass(event.target.value)}>
              <option value="inherit">inherit cluster default</option>
              <option value="standard">standard</option>
              <option value="reduced-redundancy">reduced redundancy</option>
              <option value="archive-ready">archive ready</option>
            </select>
          </label>
          <label className="space-y-2 text-sm text-slate-700">
            <span>Replica Target Override</span>
            <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={replicaTarget} onChange={(event) => setReplicaTarget(event.target.value)} placeholder="0" />
          </label>
        </div>
        <div className="mt-4 rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
          <div>Cluster default applies when the bucket class is set to inherit.</div>
          <div className="mt-1">Replica target override of 0 keeps the class-based default for this bucket.</div>
        </div>
      </section>

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">Objects</h3>
            <p className="mt-1 text-sm text-slate-500">Live object listing for this bucket.</p>
          </div>
          <div className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{items.length} objects</div>
        </div>

        <div className="mt-4 space-y-3">
          {items.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No objects are stored in this bucket yet.</div>
          ) : (
            items.map((item) => (
              <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium text-ink" title={item.key}>{item.key}</div>
                    <div className="mt-1 truncate text-sm text-slate-500" title={item.id}>{item.id}</div>
                    <div className="mt-1 text-xs text-slate-400">version {item.versionId}</div>
                    {item.legalHold ? <div className="mt-2 inline-flex rounded-full bg-rose-100 px-2 py-1 text-xs font-medium text-rose-700">legal hold</div> : null}
                  </div>
                  <div className="grid gap-2 text-sm text-slate-500 md:shrink-0 md:grid-cols-3">
                    <div>{formatBytes(item.sizeBytes)}</div>
                    <div className="truncate font-mono" title={item.etag}>{item.etag}</div>
                    <div>{formatTimestamp(item.createdAt)}</div>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </section>

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">Version History</h3>
            <p className="mt-1 text-sm text-slate-500">Inspect immutable object versions and delete markers.</p>
          </div>
          <div className="flex items-center gap-2">
            <select className="rounded-2xl border border-slate-200 px-4 py-3 text-sm" value={selectedKey} onChange={(event) => setSelectedKey(event.target.value)}>
              {Array.from(new Set(items.map((item) => item.key))).map((key) => (
                <option key={key} value={key}>
                  {key}
                </option>
              ))}
            </select>
            <button className="rounded-2xl bg-ink px-4 py-3 text-sm text-white disabled:opacity-60" type="button" onClick={() => void restoreSelected()} disabled={busy || !selectedKey}>
              {busy ? "Restoring..." : "Restore Latest"}
            </button>
          </div>
        </div>
        <div className="mt-4 space-y-3">
          {versions.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Select an object to inspect version history.</div>
          ) : (
            versions.map((item) => (
              <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium text-ink" title={item.isDeleteMarker ? "Delete Marker" : item.versionId}>{item.isDeleteMarker ? "Delete Marker" : item.versionId}</div>
                    <div className="mt-1 text-sm text-slate-500">{formatTimestamp(item.createdAt)}</div>
                  </div>
                  <div className="grid gap-2 text-sm text-slate-500 md:shrink-0 md:grid-cols-3">
                    <div>{formatBytes(item.sizeBytes)}</div>
                    <div className="font-mono break-all">{item.etag || "n/a"}</div>
                    <div>{item.isDeleteMarker ? "deleted" : "data version"}</div>
                  </div>
                </div>
                <div className="mt-3 flex items-center justify-between gap-3">
                  <div className={`rounded-full px-3 py-1 text-xs font-medium ${item.legalHold ? "bg-rose-100 text-rose-700" : "bg-slate-100 text-slate-600"}`}>
                    {item.legalHold ? "Legal hold enabled" : "No legal hold"}
                  </div>
                  {!item.isDeleteMarker ? (
                    <button
                      className="rounded-2xl bg-slate-100 px-3 py-2 text-xs text-slate-700 disabled:opacity-60"
                      type="button"
                      disabled={busy}
                      onClick={() => void toggleLegalHold(item.versionId, !item.legalHold)}
                    >
                      {busy ? "Working..." : item.legalHold ? "Release Legal Hold" : "Enable Legal Hold"}
                    </button>
                  ) : null}
                </div>
              </div>
            ))
          )}
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-2">
        <div className="rounded-3xl border border-slate-200 p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-lg font-semibold text-ink">Object Tags</h3>
              <p className="mt-1 text-sm text-slate-500">Edit version-scoped tags for the selected object.</p>
            </div>
            <button className="rounded-2xl bg-ink px-4 py-3 text-sm text-white disabled:opacity-60" type="button" onClick={() => void saveTags()} disabled={busy || !selectedKey}>
              {busy ? "Saving..." : "Save Tags"}
            </button>
          </div>
          <textarea
            className="mt-4 min-h-40 w-full rounded-2xl border border-slate-200 px-4 py-3 font-mono text-sm"
            value={tagDraft}
            onChange={(event) => setTagDraft(event.target.value)}
            placeholder={"team=platform\nclassification=internal"}
          />
        </div>

        <div className="rounded-3xl border border-slate-200 p-5">
          <div>
            <h3 className="text-lg font-semibold text-ink">Copy Object</h3>
            <p className="mt-1 text-sm text-slate-500">Create a new versioned object by copying the selected key into a new destination.</p>
          </div>
          <label className="mt-4 block space-y-2 text-sm text-slate-700">
            <span>Destination Key</span>
            <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={copyKey} onChange={(event) => setCopyKey(event.target.value)} />
          </label>
          <button className="mt-4 rounded-2xl bg-slate-100 px-4 py-3 text-sm text-slate-700 disabled:opacity-60" type="button" onClick={() => void copySelected()} disabled={busy || !selectedKey || !copyKey}>
            {busy ? "Working..." : "Copy Selected Object"}
          </button>
        </div>
      </section>
    </div>
  );
}

function parseTags(value: string) {
  const tags: Record<string, string> = {};
  for (const line of value.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    const separator = trimmed.indexOf("=");
    if (separator === -1) {
      tags[trimmed] = "";
      continue;
    }
    tags[trimmed.slice(0, separator).trim()] = trimmed.slice(separator + 1).trim();
  }
  return tags;
}

function formatTags(tags: Record<string, string>) {
  return Object.entries(tags)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-3xl border border-slate-200 p-5">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-2 break-words text-base font-medium text-ink">{value}</div>
    </div>
  );
}

function formatStorageClass(value: string) {
  switch (value) {
    case "inherit":
      return "Inherit cluster default";
    case "reduced-redundancy":
      return "Reduced redundancy";
    case "archive-ready":
      return "Archive ready";
    default:
      return "Standard";
  }
}
