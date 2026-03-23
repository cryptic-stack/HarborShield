import { useEffect, useState } from "react";
import { api, extractErrorMessage } from "../api/client";
import { formatBytes } from "../lib/format";

type Bucket = { id: string; name: string };
type ObjectItem = { id: string; key: string; sizeBytes: number; contentType: string; scanStatus: string };
type UploadStatus = "queued" | "uploading" | "uploaded" | "failed";
type UploadEntry = {
  id: string;
  file: File;
  objectKey: string;
  status: UploadStatus;
  error: string;
};

const tones: Record<string, string> = {
  "pending-scan": "bg-amber-100 text-amber-800",
  clean: "bg-emerald-100 text-emerald-700",
  infected: "bg-rose-100 text-rose-700",
  "scan-failed": "bg-slate-100 text-slate-700",
};

const uploadTones: Record<UploadStatus, string> = {
  queued: "bg-slate-100 text-slate-700",
  uploading: "bg-sky-100 text-sky-700",
  uploaded: "bg-emerald-100 text-emerald-700",
  failed: "bg-rose-100 text-rose-700",
};

export function UploadsPage() {
  const [buckets, setBuckets] = useState<Bucket[]>([]);
  const [bucketId, setBucketId] = useState("");
  const [items, setItems] = useState<ObjectItem[]>([]);
  const [keyPrefix, setKeyPrefix] = useState("");
  const [uploads, setUploads] = useState<UploadEntry[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function loadBuckets() {
    const result = await api<{ items: Bucket[] }>("/buckets");
    const bucketItems = result.items ?? [];
    setBuckets(bucketItems);
    if (!bucketId && bucketItems[0]) {
      setBucketId(bucketItems[0].id);
    }
  }

  async function loadObjects(nextBucketId: string) {
    if (!nextBucketId) {
      setItems([]);
      return;
    }
    const result = await api<{ items: ObjectItem[] }>(`/buckets/${nextBucketId}/objects`);
    setItems(result.items ?? []);
  }

  function setUploadStatus(id: string, status: UploadStatus, nextError = "") {
    setUploads((current) =>
      current.map((entry) =>
        entry.id === id
          ? {
              ...entry,
              status,
              error: nextError,
            }
          : entry,
      ),
    );
  }

  function clearCompletedUploads() {
    setUploads((current) => current.filter((entry) => entry.status === "queued" || entry.status === "uploading" || entry.status === "failed"));
  }

  function getSessionToken(): HeadersInit {
    const stored = window.localStorage.getItem("harborshield-session");
    if (!stored) {
      return {};
    }

    try {
      const parsed = JSON.parse(stored) as { accessToken?: string } | null;
      if (!parsed?.accessToken) {
        return {};
      }
      return { Authorization: `Bearer ${parsed.accessToken}` };
    } catch {
      window.localStorage.removeItem("harborshield-session");
      return {};
    }
  }

  function buildObjectKey(prefix: string, fileName: string) {
    const trimmed = prefix.trim();
    if (!trimmed) {
      return fileName;
    }
    if (uploads.length === 1) {
      return trimmed;
    }
    return `${trimmed.replace(/\/+$/, "")}/${fileName}`;
  }

  function handleFileSelection(fileList: FileList | null) {
    const nextFiles = Array.from(fileList ?? []);
    setUploads(
      nextFiles.map((file) => ({
        id: `${file.name}-${file.lastModified}-${file.size}`,
        file,
        objectKey: "",
        status: "queued",
        error: "",
      })),
    );
  }

  async function uploadObjects() {
    if (!bucketId || uploads.length === 0) {
      setError("Choose a bucket and at least one file before uploading");
      return;
    }
    setBusy(true);
    setError("");

    const authHeaders = getSessionToken();
    const queuedUploads = uploads.map((entry) => ({
      ...entry,
      objectKey: buildObjectKey(keyPrefix, entry.file.name),
    }));
    setUploads(queuedUploads);

    try {
      for (const entry of queuedUploads) {
        setUploadStatus(entry.id, "uploading");
        const form = new FormData();
        form.set("file", entry.file);
        form.set("key", entry.objectKey);
        const response = await fetch(`/api/v1/buckets/${bucketId}/objects/upload`, {
          method: "POST",
          headers: authHeaders,
          body: form,
        });
        if (!response.ok) {
          setUploadStatus(entry.id, "failed", extractErrorMessage(await response.text(), response.statusText));
          continue;
        }
        setUploadStatus(entry.id, "uploaded");
      }
      await loadObjects(bucketId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setBusy(false);
    }
  }

  async function deleteObject(objectKey: string) {
    setBusy(true);
    setError("");
    try {
      await api(`/buckets/${bucketId}/objects?key=${encodeURIComponent(objectKey)}`, { method: "DELETE" });
      await loadObjects(bucketId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setBusy(false);
    }
  }

  async function downloadObject(objectKey: string) {
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/buckets/${bucketId}/objects/download?key=${encodeURIComponent(objectKey)}`, {
        headers: getSessionToken(),
      });
      if (!response.ok) {
        throw new Error(extractErrorMessage(await response.text(), response.statusText));
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = objectKey.split("/").pop() || objectKey;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Download failed");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void loadBuckets();
  }, []);

  useEffect(() => {
    if (!bucketId) {
      return;
    }
    void loadObjects(bucketId);
  }, [bucketId]);

  const uploadedCount = uploads.filter((entry) => entry.status === "uploaded").length;
  const failedCount = uploads.filter((entry) => entry.status === "failed").length;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Upload Manager</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Upload one or many objects directly from the admin plane while the S3-compatible surface continues to serve service credentials.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      <section className="rounded-3xl border border-slate-200 p-5">
        <h3 className="text-lg font-semibold text-ink">Upload Objects</h3>
        <div className="mt-4 grid gap-3 lg:grid-cols-[220px_1fr_1fr_auto]">
          <select className="rounded-2xl border border-slate-200 px-4 py-3" value={bucketId} onChange={(event) => setBucketId(event.target.value)}>
            {buckets.map((bucket) => (
              <option key={bucket.id} value={bucket.id}>
                {bucket.name}
              </option>
            ))}
          </select>
          <input
            className="rounded-2xl border border-slate-200 px-4 py-3"
            placeholder="optional key prefix or single-file key"
            value={keyPrefix}
            onChange={(event) => setKeyPrefix(event.target.value)}
          />
          <input
            className="rounded-2xl border border-slate-200 px-4 py-3"
            type="file"
            multiple
            onChange={(event) => handleFileSelection(event.target.files)}
          />
          <button className="rounded-2xl bg-ink px-5 py-3 text-white disabled:opacity-60" disabled={busy || uploads.length === 0} onClick={() => void uploadObjects()} type="button">
            {busy ? "Uploading..." : `Upload ${uploads.length || ""}`.trim()}
          </button>
        </div>

        <div className="mt-4 space-y-3">
          {uploads.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Select one or more files to build an upload queue.</div>
          ) : (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="text-sm text-slate-500">
                  {uploads.length} queued • {uploadedCount} uploaded • {failedCount} failed
                </div>
                <button className="rounded-2xl bg-slate-100 px-4 py-2 text-sm text-slate-700" onClick={() => clearCompletedUploads()} type="button">
                  Clear Completed
                </button>
              </div>
              {uploads.map((entry) => (
                <div key={entry.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-ink" title={entry.objectKey || entry.file.name}>
                        {entry.objectKey || entry.file.name}
                      </div>
                      <div className="mt-1 text-sm text-slate-500">{entry.file.name} • {formatBytes(entry.file.size)}</div>
                      {entry.error ? <div className="mt-2 text-xs text-rose-600">{entry.error}</div> : null}
                    </div>
                    <div className="flex items-center gap-2 lg:shrink-0">
                      <div className={`rounded-full px-3 py-1 text-xs ${uploadTones[entry.status]}`}>{entry.status}</div>
                    </div>
                  </div>
                </div>
              ))}
            </>
          )}
        </div>
      </section>

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">Uploaded Objects</h3>
            <p className="mt-1 text-sm text-slate-500">Use the admin plane for quick validation, downloads, and cleanup.</p>
          </div>
          <div className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{items.length} objects</div>
        </div>
        <div className="mt-4 space-y-3">
          {items.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No objects in the selected bucket yet.</div>
          ) : (
            items.map((item) => (
              <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium text-ink" title={item.key}>{item.key}</div>
                    <div className="mt-1 text-sm text-slate-500">{formatBytes(item.sizeBytes)} • {item.contentType}</div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 lg:shrink-0">
                    <div className={`rounded-full px-3 py-1 text-xs ${tones[item.scanStatus] ?? "bg-slate-100 text-slate-700"}`}>{item.scanStatus}</div>
                    <button className="rounded-2xl bg-slate-100 px-4 py-2 text-sm text-slate-700" onClick={() => downloadObject(item.key)} type="button">
                      Download
                    </button>
                    <button className="rounded-2xl bg-rose-600 px-4 py-2 text-sm text-white" onClick={() => void deleteObject(item.key)} type="button">
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
