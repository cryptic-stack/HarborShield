import { useEffect, useState } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

type StorageNode = {
  id: string;
  name: string;
  endpoint: string;
  backendType: string;
  zone: string;
  status: string;
  operatorState: string;
  capacityBytes: number;
  usedBytes: number;
  metadata?: Record<string, unknown>;
  lastHeartbeatAt?: string;
};

type Placement = {
  id: string;
  objectId: string;
  bucketId: string;
  objectKey: string;
  versionId: string;
  replicaIndex: number;
  chunkOrdinal: number;
  nodeName?: string;
  locator: string;
  state: string;
};

type SettingsSnapshot = {
  storageBackend: string;
  storageDistributedReplicas: number;
  storageDefaultClass: string;
  storageClassPolicies: { name: string; label: string; description: string; defaultReplicas: number }[];
};

type DashboardSummary = {
  storageNodeCount: number;
  replicaTarget: number;
  offlineStorageNodes: number;
  degradedPlacements: number;
  rebalanceGapCount: number;
};

type MigrationStatus = {
  pendingLocalObjects: number;
  pendingLocalBytes: number;
  distributedObjects: number;
  distributedBytes: number;
};

type MigrationHistoryItem = {
  actor: string;
  action: string;
  outcome: string;
  createdAt: string;
  detail?: {
    migratedCount?: number;
    pendingLocalObjects?: number;
    pendingLocalBytes?: number;
    distributedObjects?: number;
    distributedBytes?: number;
  };
};

export function StoragePage() {
  const [nodes, setNodes] = useState<StorageNode[]>([]);
  const [placements, setPlacements] = useState<Placement[]>([]);
  const [settings, setSettings] = useState<SettingsSnapshot | null>(null);
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [migrationStatus, setMigrationStatus] = useState<MigrationStatus | null>(null);
  const [migrationHistory, setMigrationHistory] = useState<MigrationHistoryItem[]>([]);
  const [keyFilter, setKeyFilter] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [updatingNodeId, setUpdatingNodeId] = useState("");
  const [repinningNodeId, setRepinningNodeId] = useState("");
  const [migrating, setMigrating] = useState(false);
  const [newNodeName, setNewNodeName] = useState("");
  const [newNodeEndpoint, setNewNodeEndpoint] = useState("");
  const [newNodeZone, setNewNodeZone] = useState("");

  const load = () => {
    const placementQuery = keyFilter.trim() ? `/storage/placements?limit=20&key=${encodeURIComponent(keyFilter.trim())}` : "/storage/placements?limit=20";
    return Promise.all([
      api<{ items?: StorageNode[] }>("/storage/nodes"),
      api<{ items?: Placement[] }>(placementQuery),
      api<SettingsSnapshot>("/settings"),
      api<DashboardSummary>("/dashboard"),
      api<MigrationStatus>("/storage/migration-status"),
      api<{ items?: MigrationHistoryItem[] }>("/storage/migrations/history?limit=10"),
    ])
      .then(([nodeResult, placementResult, settingsResult, dashboardResult, migrationResult, historyResult]) => {
        setNodes(nodeResult.items ?? []);
        setPlacements(placementResult.items ?? []);
        setSettings(settingsResult);
        setSummary(dashboardResult);
        setMigrationStatus(migrationResult);
        setMigrationHistory(historyResult.items ?? []);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load storage topology"));
  };

  useEffect(() => {
    void load();
  }, [keyFilter]);

  const updateNodeState = async (nodeId: string, operatorState: string) => {
    setUpdatingNodeId(nodeId);
    setError("");
    setNotice("");
    try {
      await api(`/storage/nodes/${nodeId}`, {
        method: "PATCH",
        body: JSON.stringify({ operatorState }),
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update node state");
    } finally {
      setUpdatingNodeId("");
    }
  };

  const registerNode = async () => {
    if (!newNodeName.trim() || !newNodeEndpoint.trim()) {
      setError("Node name and endpoint are required.");
      return;
    }
    setError("");
    setNotice("");
    try {
      await api("/storage/nodes", {
        method: "POST",
        body: JSON.stringify({
          name: newNodeName.trim(),
          endpoint: newNodeEndpoint.trim(),
          zone: newNodeZone.trim(),
          operatorState: "maintenance",
        }),
      });
      setNewNodeName("");
      setNewNodeEndpoint("");
      setNewNodeZone("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to register storage node");
    }
  };

  const repinNodeTLSIdentity = async (nodeId: string) => {
    setRepinningNodeId(nodeId);
    setError("");
    setNotice("");
    try {
      await api(`/storage/nodes/${nodeId}/tls/re-pin`, { method: "POST" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to re-pin node TLS identity");
    } finally {
      setRepinningNodeId("");
    }
  };

  const migrateLocalObjects = async () => {
    setMigrating(true);
    setError("");
    setNotice("");
    try {
      const result = await api<{ migratedCount: number; pendingLocalObjects: number; pendingLocalBytes: number; distributedObjects: number; distributedBytes: number }>(
        "/storage/migrations/local-to-distributed",
        {
        method: "POST",
        body: JSON.stringify({ limit: 100 }),
        },
      );
      setNotice(
        result.migratedCount > 0
          ? `Migrated ${result.migratedCount} objects. ${result.pendingLocalObjects} local objects remain (${formatBytes(result.pendingLocalBytes)} still on local storage).`
          : "No local objects were migrated in this pass.",
      );
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to migrate local objects");
    } finally {
      setMigrating(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Storage</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Review storage-node health, mirrored placement records, and the current backend topology.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}
      {notice ? <div className="rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{notice}</div> : null}

      <section className="grid gap-4 md:grid-cols-3">
        <MetricCard label="Configured Nodes" value={String(nodes.length)} />
        <MetricCard label="Healthy Nodes" value={String(nodes.filter((node) => node.status === "healthy").length)} />
        <MetricCard label="Replica Target" value={String(summary?.replicaTarget ?? settings?.storageDistributedReplicas ?? 0)} />
        <MetricCard label="Degraded Placements" value={String(summary?.degradedPlacements ?? 0)} />
        <MetricCard label="Rebalance Gaps" value={String(summary?.rebalanceGapCount ?? 0)} />
        <MetricCard label="Recent Placements" value={String(placements.length)} />
      </section>

      {settings?.storageBackend === "distributed" ? (
        <section className="rounded-3xl border border-slate-200 p-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="max-w-2xl">
              <h3 className="text-lg font-semibold text-ink">Local Object Migration</h3>
              <p className="mt-1 text-sm text-slate-500">
                Migrate older locally stored objects into the live distributed node set after the cluster is already running. This lets us adopt remote storage without
                editing backend env vars and restarting the stack.
              </p>
            </div>
            <button
              type="button"
              onClick={() => void migrateLocalObjects()}
              disabled={migrating || (migrationStatus?.pendingLocalObjects ?? 0) === 0}
              className="rounded-full bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {migrating ? "Migrating..." : "Migrate 100 Local Objects"}
            </button>
          </div>
          <div
            className={`mt-4 rounded-2xl border px-4 py-3 text-sm ${
              isLocalDrainComplete(nodes, migrationStatus)
                ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                : "border-amber-200 bg-amber-50 text-amber-800"
            }`}
          >
            {isLocalDrainComplete(nodes, migrationStatus)
              ? "Local drain complete. New writes can use the active distributed node set, and there are no remaining local objects waiting for migration."
              : `Local drain still in progress. ${migrationStatus?.pendingLocalObjects ?? 0} objects (${formatBytes(
                  migrationStatus?.pendingLocalBytes ?? 0,
                )}) still need migration before local storage is fully drained.`}
          </div>
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            <PolicyCard title="Pending Local Objects" value={String(migrationStatus?.pendingLocalObjects ?? 0)} />
            <PolicyCard title="Pending Local Bytes" value={formatBytes(migrationStatus?.pendingLocalBytes ?? 0)} />
            <PolicyCard title="Distributed Objects" value={String(migrationStatus?.distributedObjects ?? 0)} />
            <PolicyCard title="Distributed Bytes" value={formatBytes(migrationStatus?.distributedBytes ?? 0)} />
          </div>
          <div className="mt-4 space-y-3">
            <h4 className="text-sm font-semibold text-ink">Recent Migration Runs</h4>
            {migrationHistory.length ? (
              migrationHistory.map((item, index) => (
                <div key={`${item.createdAt}-${index}`} className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-600">
                  <div className="font-medium text-ink">
                    {item.detail?.migratedCount ?? 0} objects migrated by {item.actor || "operator"}
                  </div>
                  <div className="mt-1">
                    {new Date(item.createdAt).toLocaleString()} | remaining local objects: {item.detail?.pendingLocalObjects ?? 0} (
                    {formatBytes(item.detail?.pendingLocalBytes ?? 0)})
                  </div>
                </div>
              ))
            ) : (
              <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No migration runs recorded yet.</div>
            )}
          </div>
        </section>
      ) : null}

      <section className="rounded-3xl border border-slate-200 p-5">
        <h3 className="text-lg font-semibold text-ink">Placement Policy</h3>
        <div className="mt-4 grid gap-3 md:grid-cols-3">
          <PolicyCard title="Backend" value={settings?.storageBackend === "distributed" ? "Distributed" : "Local"} />
          <PolicyCard title="Replica Target" value={String(summary?.replicaTarget ?? settings?.storageDistributedReplicas ?? 0)} />
          <PolicyCard title="Default Storage Class" value={formatStorageClass(settings?.storageDefaultClass ?? "standard")} />
          <PolicyCard
            title="Operator Signal"
            value={
              (summary?.degradedPlacements ?? 0) === 0 && (summary?.rebalanceGapCount ?? 0) === 0
                ? "Replica policy currently satisfied"
                : `${summary?.degradedPlacements ?? 0} degraded placements and ${summary?.rebalanceGapCount ?? 0} rebalance gaps`
            }
          />
        </div>
        {settings?.storageClassPolicies?.length ? (
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            {settings.storageClassPolicies.map((policy) => (
              <PolicyCard key={policy.name} title={`${policy.label} (${policy.defaultReplicas})`} value={policy.description} />
            ))}
          </div>
        ) : null}
      </section>

      {settings?.storageBackend === "distributed" ? (
        <section className="rounded-3xl border border-slate-200 p-5">
          <h3 className="text-lg font-semibold text-ink">Node Admission</h3>
          <p className="mt-1 text-sm text-slate-500">
            Register a new storage node in maintenance mode before activating it for replica placement. HarborShield now reads the distributed node catalog live from the
            control plane, so endpoint changes here do not require an API or worker restart.
          </p>
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            <input
              value={newNodeName}
              onChange={(event) => setNewNodeName(event.target.value)}
              placeholder="blobnode-d"
              className="rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-orange-500"
            />
            <input
              value={newNodeEndpoint}
              onChange={(event) => setNewNodeEndpoint(event.target.value)}
              placeholder="http://blobnode-d:9100"
              className="rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-orange-500"
            />
            <input
              value={newNodeZone}
              onChange={(event) => setNewNodeZone(event.target.value)}
              placeholder="zone-a"
              className="rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-orange-500"
            />
          </div>
          <button
            type="button"
            onClick={() => void registerNode()}
            className="mt-4 rounded-full bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-700"
          >
            Register Maintenance Node
          </button>
        </section>
      ) : null}

      <section className="space-y-3">
        <h3 className="text-lg font-semibold text-ink">Node Health</h3>
        {nodes.length ? (
          <div className="grid gap-4 lg:grid-cols-3">
            {nodes.map((node) => (
              <div key={node.id} className="rounded-3xl border border-slate-200 p-5">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-sm font-semibold text-ink">{node.name}</div>
                    <div className="mt-1 break-all text-xs text-slate-500">{node.endpoint}</div>
                  </div>
                  <span
                    className={`rounded-full px-3 py-1 text-xs font-medium ${
                      node.status === "healthy" ? "bg-emerald-100 text-emerald-700" : "bg-amber-100 text-amber-700"
                    }`}
                  >
                    {node.status}
                  </span>
                </div>
                <div className="mt-4 space-y-2 text-sm text-slate-600">
                  <div>Operator state: {node.operatorState}</div>
                  <div>Backend: {node.backendType}</div>
                  <div>Zone: {node.zone || "default"}</div>
                  <div>Used: {formatBytes(node.usedBytes)}</div>
                  <div>Capacity: {node.capacityBytes > 0 ? formatBytes(node.capacityBytes) : "not reported"}</div>
                  <div>Last heartbeat: {node.lastHeartbeatAt || "not yet reported"}</div>
                  <div>TLS identity: {formatTLSIdentity(node.metadata)}</div>
                  {node.metadata?.tlsCommonName ? <div>TLS common name: {String(node.metadata.tlsCommonName)}</div> : null}
                  {node.metadata?.tlsObservedFingerprintSha256 ? (
                    <div className="break-all text-xs text-slate-500">TLS fingerprint: {String(node.metadata.tlsObservedFingerprintSha256)}</div>
                  ) : null}
                </div>
                {settings?.storageBackend === "distributed" ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {["active", "draining", "maintenance"].map((state) => (
                      <button
                        key={state}
                        type="button"
                        onClick={() => void updateNodeState(node.id, state)}
                        disabled={updatingNodeId === node.id || node.operatorState === state}
                        className={`rounded-full px-3 py-1 text-xs font-medium transition ${
                          node.operatorState === state
                            ? "bg-slate-900 text-white"
                            : "border border-slate-300 bg-white text-slate-700 hover:border-orange-400"
                        } disabled:cursor-not-allowed disabled:opacity-60`}
                      >
                        {updatingNodeId === node.id && node.operatorState !== state ? "Updating..." : state}
                      </button>
                    ))}
                    {canRepinTLSIdentity(node.metadata) ? (
                      <button
                        type="button"
                        onClick={() => void repinNodeTLSIdentity(node.id)}
                        disabled={repinningNodeId === node.id}
                        className="rounded-full border border-orange-300 bg-orange-50 px-3 py-1 text-xs font-medium text-orange-700 transition hover:border-orange-400 hover:bg-orange-100 disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        {repinningNodeId === node.id ? "Re-pinning..." : "Re-pin TLS Identity"}
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No storage nodes are configured for this deployment.</div>
        )}
      </section>

      <section className="space-y-3">
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <h3 className="text-lg font-semibold text-ink">Recent Placements</h3>
            <p className="mt-1 text-sm text-slate-500">Filter by object key to inspect mirrored placement records.</p>
          </div>
          <label className="block">
            <span className="mb-1 block text-xs uppercase tracking-[0.2em] text-slate-500">Object Key Filter</span>
            <input
              value={keyFilter}
              onChange={(event) => setKeyFilter(event.target.value)}
              placeholder="demo.txt"
              className="w-full rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-orange-500 md:w-72"
            />
          </label>
        </div>
        {placements.length ? (
          <div className="overflow-hidden rounded-3xl border border-slate-200">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50">
                <tr className="text-left text-slate-500">
                  <th className="px-4 py-3 font-medium">Object</th>
                  <th className="px-4 py-3 font-medium">Version</th>
                  <th className="px-4 py-3 font-medium">Replica</th>
                  <th className="px-4 py-3 font-medium">Node</th>
                  <th className="px-4 py-3 font-medium">State</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {placements.map((placement) => (
                  <tr key={placement.id}>
                    <td className="px-4 py-3">
                      <div className="max-w-[22rem] truncate text-ink" title={placement.objectKey}>
                        {placement.objectKey}
                      </div>
                      <div className="text-xs text-slate-500">{placement.locator}</div>
                    </td>
                    <td className="px-4 py-3 text-slate-600">{placement.versionId || "current"}</td>
                    <td className="px-4 py-3 text-slate-600">{placement.replicaIndex}</td>
                    <td className="px-4 py-3 text-slate-600">{placement.nodeName || "unassigned"}</td>
                    <td className="px-4 py-3 text-slate-600">{placement.state}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No placement records have been written yet.</div>
        )}
      </section>
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

function formatTLSIdentity(metadata?: Record<string, unknown>) {
  const status = String(metadata?.tlsIdentityStatus ?? "");
  switch (status) {
    case "verified":
      return "Verified";
    case "pinned":
      return "Pinned on first contact";
    case "mismatch":
      return "Certificate mismatch detected";
    default:
      return metadata?.tlsObservedFingerprintSha256 ? "Observed" : "Not using TLS pinning";
  }
}

function canRepinTLSIdentity(metadata?: Record<string, unknown>) {
  return typeof metadata?.tlsObservedFingerprintSha256 === "string" && metadata.tlsObservedFingerprintSha256.length > 0;
}

function isLocalDrainComplete(nodes: StorageNode[], migrationStatus: MigrationStatus | null) {
  return (migrationStatus?.pendingLocalObjects ?? 0) === 0 && nodes.some((node) => node.operatorState === "active" && node.status === "healthy");
}

function MetricCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-3xl border border-slate-200 p-5">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-2 text-2xl font-semibold text-ink">{value}</div>
    </div>
  );
}

function PolicyCard({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-2xl bg-slate-50 p-4">
      <div className="text-sm font-medium text-ink">{title}</div>
      <p className="mt-2 text-sm text-slate-600">{value}</p>
    </div>
  );
}
