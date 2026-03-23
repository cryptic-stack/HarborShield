import { useEffect, useState } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

type Bucket = { id: string; name: string };
type ObjectItem = { id: string; key: string; sizeBytes: number; etag: string };

export function ObjectsPage() {
  const [buckets, setBuckets] = useState<Bucket[]>([]);
  const [bucketID, setBucketID] = useState("");
  const [items, setItems] = useState<ObjectItem[]>([]);

  useEffect(() => {
    void api<{ items: Bucket[] }>("/buckets").then((result) => {
      const bucketItems = result.items ?? [];
      setBuckets(bucketItems);
      if (bucketItems[0]) {
        setBucketID(bucketItems[0].id);
      }
    });
  }, []);

  useEffect(() => {
    if (!bucketID) return;
    void api<{ items: ObjectItem[] }>(`/buckets/${bucketID}/objects`).then((result) => setItems(result.items ?? []));
  }, [bucketID]);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Object Browser</h2>
        <p className="mt-1 text-sm text-slate-600">Browse metadata-backed objects stored on disk.</p>
      </div>
      <select className="rounded-2xl border border-slate-200 px-4 py-3" value={bucketID} onChange={(e) => setBucketID(e.target.value)}>
        {buckets.map((bucket) => (
          <option key={bucket.id} value={bucket.id}>
            {bucket.name}
          </option>
        ))}
      </select>
      <div className="grid gap-3">
        {items.map((item) => (
          <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
            <div className="truncate font-medium" title={item.key}>{item.key}</div>
            <div className="mt-1 text-sm text-slate-500">{formatBytes(item.sizeBytes)}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
