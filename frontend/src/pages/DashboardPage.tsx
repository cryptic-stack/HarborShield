import { useEffect, useState } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

type DashboardSummary = {
  bucketCount: number;
  liveObjectCount: number;
  totalStoredBytes: number;
  pendingScanCount: number;
  deadLetterCount: number;
  storageNodeCount: number;
  replicaTarget: number;
  offlineStorageNodes: number;
  degradedPlacements: number;
  rebalanceGapCount: number;
  recentAuditCount24h: number;
  latestAuditAt: string;
};

export function DashboardPage() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    void api<DashboardSummary>("/dashboard")
      .then(setSummary)
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load dashboard"));
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <div className="text-xs uppercase tracking-[0.3em] text-ember">Overview</div>
        <h2 className="mt-2 text-3xl font-semibold text-ink">Storage at a glance</h2>
        <p className="mt-2 max-w-2xl text-sm text-slate-600">
          Track the live shape of the platform: active buckets and objects, stored bytes, scan backlog, delivery trouble, storage-node health, and recent control-plane activity.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      {summary ? (
        <>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <MetricCard label="Buckets" value={String(summary.bucketCount)} detail="Active namespaces" />
            <MetricCard label="Live Objects" value={String(summary.liveObjectCount)} detail="Current non-deleted latest objects" />
            <MetricCard label="Stored Bytes" value={formatBytes(summary.totalStoredBytes)} detail="Total live object footprint" />
            <MetricCard label="Pending Scans" value={String(summary.pendingScanCount)} detail="Objects still waiting for malware scan" />
            <MetricCard label="Dead Letters" value={String(summary.deadLetterCount)} detail="Webhook deliveries needing operator review" />
            <MetricCard label="Storage Nodes" value={String(summary.storageNodeCount)} detail="Configured distributed storage nodes" />
            <MetricCard label="Replica Target" value={String(summary.replicaTarget)} detail="Desired distributed replicas per object" />
            <MetricCard label="Offline Nodes" value={String(summary.offlineStorageNodes)} detail="Storage nodes currently not healthy" />
            <MetricCard label="Degraded Placements" value={String(summary.degradedPlacements)} detail="Mirrored objects below the healthy placement target" />
            <MetricCard label="Rebalance Gaps" value={String(summary.rebalanceGapCount)} detail="Objects missing expected placement rows" />
            <MetricCard label="Recent Audit Events" value={String(summary.recentAuditCount24h)} detail="Audit entries in the last 24 hours" />
          </div>

          <section className="rounded-3xl border border-slate-200 p-5">
            <h3 className="text-lg font-semibold text-ink">Operational Notes</h3>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              <DetailCard
                title="Latest Audit Activity"
                text={summary.latestAuditAt ? new Date(summary.latestAuditAt).toLocaleString() : "No audit records yet"}
              />
              <DetailCard
                title="Storage Footprint"
                text={
                  summary.liveObjectCount === 0
                    ? "No live objects stored yet."
                    : `${formatBytes(summary.totalStoredBytes)} across ${summary.liveObjectCount} live objects`
                }
              />
              <DetailCard
                title="Queue Pressure"
                text={
                  summary.pendingScanCount === 0 && summary.deadLetterCount === 0
                    ? "No scan backlog or dead-letter deliveries right now."
                    : `${summary.pendingScanCount} pending scans and ${summary.deadLetterCount} dead-letter deliveries need review`
                }
              />
              <DetailCard
                title="Storage Health"
                text={
                  summary.storageNodeCount === 0
                    ? "Local storage mode is active."
                    : summary.offlineStorageNodes === 0 && summary.degradedPlacements === 0 && summary.rebalanceGapCount === 0
                      ? `All ${summary.storageNodeCount} storage nodes are healthy and the ${summary.replicaTarget}-replica policy is satisfied.`
                      : `${summary.offlineStorageNodes} offline nodes, ${summary.degradedPlacements} degraded placements, and ${summary.rebalanceGapCount} rebalance gaps need review.`
                }
              />
            </div>
          </section>
        </>
      ) : (
        <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Loading dashboard summary...</div>
      )}
    </div>
  );
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="rounded-3xl border border-slate-200 p-5">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-2 text-2xl font-semibold text-ink">{value}</div>
      <p className="mt-2 text-sm text-slate-600">{detail}</p>
    </div>
  );
}

function DetailCard({ title, text }: { title: string; text: string }) {
  return (
    <div className="rounded-2xl bg-slate-50 p-4">
      <div className="text-sm font-medium text-ink">{title}</div>
      <p className="mt-2 text-sm text-slate-600">{text}</p>
    </div>
  );
}
