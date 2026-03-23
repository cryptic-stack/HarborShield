import { useMemo, useState } from "react";
import type { DeploymentSetupStatus } from "../hooks/useSession";

type SetupPageProps = {
  userEmail: string;
  status: DeploymentSetupStatus;
  onSubmit: (input: {
    mode: "single-node" | "distributed";
    distributedMode?: "local" | "remote";
    remoteEndpoints?: string[];
  }) => Promise<void>;
};

function isValidEndpoint(value: string) {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") && Boolean(parsed.host) && !parsed.search && !parsed.hash;
  } catch {
    return false;
  }
}

export function SetupPage({ userEmail, status, onSubmit }: SetupPageProps) {
  const savedRemoteEndpoints = Array.isArray(status.remoteEndpoints) ? status.remoteEndpoints : [];
  const [mode, setMode] = useState<"single-node" | "distributed">(
    status.desiredStorageBackend === "distributed" ? "distributed" : "single-node",
  );
  const [distributedMode, setDistributedMode] = useState<"local" | "remote">(
    status.distributedScope === "remote" ? "remote" : "local",
  );
  const [remoteEndpointsText, setRemoteEndpointsText] = useState(savedRemoteEndpoints.join("\n"));
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const remoteEndpoints = useMemo(
    () =>
      remoteEndpointsText
        .split(/\r?\n/)
        .map((value) => value.trim().replace(/\/+$/, ""))
        .filter(Boolean),
    [remoteEndpointsText],
  );

  const invalidEndpoints = useMemo(
    () => remoteEndpoints.filter((value) => !isValidEndpoint(value)),
    [remoteEndpoints],
  );

  const planSummary =
    mode === "single-node"
      ? "Single node on the local encrypted filesystem"
      : distributedMode === "local"
        ? "Distributed with local blob nodes"
        : "Distributed with remote blob nodes";

  const nextAction = useMemo(() => {
    if (!status.applyRequired) {
      return null;
    }
    if (status.desiredStorageBackend === "distributed" && status.distributedScope === "local") {
      return {
        title: "Apply the distributed local runtime",
        body: "Your deployment plan is saved. Restart HarborShield with the distributed Compose profile before continuing.",
        command: "docker compose --profile distributed --env-file .env up --build -d",
      };
    }
    if (status.desiredStorageBackend === "distributed" && status.distributedScope === "remote") {
      return {
        title: "Apply the remote distributed runtime",
        body: "Your deployment plan is saved. Update the runtime to use the distributed backend and the saved remote node endpoints, then restart the stack.",
        command: `STORAGE_BACKEND=distributed\nSTORAGE_DISTRIBUTED_ENDPOINTS=${savedRemoteEndpoints.join(",")}`,
      };
    }
    return {
      title: "Return to the single-node runtime",
      body: "Your deployment plan is saved. Restart HarborShield without the distributed profile so the running stack matches the saved single-node plan.",
      command: "docker compose --env-file .env up --build -d",
    };
  }, [savedRemoteEndpoints, status]);

  async function submit() {
    setError("");
    if (mode === "distributed" && distributedMode === "remote") {
      if (remoteEndpoints.length === 0) {
        setError("Enter at least one remote node endpoint for a remote distributed deployment.");
        return;
      }
      if (invalidEndpoints.length > 0) {
        setError(`Use full HTTP or HTTPS URLs for every remote endpoint. Invalid entries: ${invalidEndpoints.join(", ")}`);
        return;
      }
    }
    setSaving(true);
    try {
      await onSubmit({
        mode,
        distributedMode: mode === "distributed" ? distributedMode : undefined,
        remoteEndpoints: mode === "distributed" && distributedMode === "remote" ? remoteEndpoints : [],
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save deployment setup.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-6">
      <div className="grid w-full max-w-6xl gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
        <section className="rounded-[2rem] border border-slate-200 bg-white p-8 shadow-2xl">
          <div className="text-xs uppercase tracking-[0.35em] text-ember">{status.completed ? "Setup Review" : "First Run"}</div>
          <h1 className="mt-3 text-3xl font-semibold text-ink">{status.applyRequired ? "Apply Deployment Plan" : "Deployment Setup"}</h1>
          <p className="mt-2 max-w-3xl text-sm text-slate-600">
            {status.applyRequired
              ? "HarborShield saved your deployment choice. Apply the matching runtime before you continue so the control plane and storage topology stay in sync."
              : "Choose how this HarborShield deployment should operate before you start creating data. You can keep it single-node, or prepare it for distributed storage and remote nodes."}
          </p>

          {status.completed ? (
            <div className="mt-5 rounded-3xl border border-emerald-200 bg-emerald-50 px-5 py-4 text-sm text-emerald-800">
              Saved plan: <span className="font-medium">{planSummary}</span>
            </div>
          ) : null}

          <div className="mt-6 grid gap-4 lg:grid-cols-2">
            <ChoiceCard
              title="Single Node"
              active={mode === "single-node"}
              description="Use local disk on this host. This is the simplest and safest first-run choice."
              onClick={() => setMode("single-node")}
            />
            <ChoiceCard
              title="Distributed"
              active={mode === "distributed"}
              description="Prepare the control plane for replicated storage nodes. You can keep those nodes local or point to remote endpoints."
              onClick={() => setMode("distributed")}
            />
          </div>

          {mode === "distributed" ? (
            <div className="mt-6 space-y-4 rounded-3xl border border-slate-200 p-5">
              <h2 className="text-lg font-semibold text-ink">Distributed Topology</h2>
              <div className="grid gap-4 md:grid-cols-2">
                <ChoiceCard
                  title="Local Nodes"
                  active={distributedMode === "local"}
                  description="Use locally hosted storage nodes, such as the Compose distributed profile."
                  compact
                  onClick={() => setDistributedMode("local")}
                />
                <ChoiceCard
                  title="Remote Nodes"
                  active={distributedMode === "remote"}
                  description="Point HarborShield at remote blob nodes that are reachable over the network."
                  compact
                  onClick={() => setDistributedMode("remote")}
                />
              </div>

              {distributedMode === "remote" ? (
                <label className="block">
                  <span className="mb-2 block text-sm font-medium text-slate-700">Remote Node Endpoints</span>
                  <textarea
                    value={remoteEndpointsText}
                    onChange={(event) => setRemoteEndpointsText(event.target.value)}
                    rows={6}
                    placeholder={"https://node-a.example.com:9100\nhttps://node-b.example.com:9100"}
                    className="w-full rounded-2xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-orange-500"
                  />
                  <p className="mt-2 text-xs text-slate-500">Enter one full HTTP or HTTPS endpoint per line. Queries, fragments, and duplicates are rejected.</p>
                  {remoteEndpoints.length > 0 ? (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {remoteEndpoints.map((endpoint) => (
                        <span
                          key={endpoint}
                          className={`rounded-full px-3 py-1 text-xs ${
                            isValidEndpoint(endpoint) ? "bg-emerald-50 text-emerald-700" : "bg-rose-50 text-rose-700"
                          }`}
                        >
                          {endpoint}
                        </span>
                      ))}
                    </div>
                  ) : null}
                </label>
              ) : (
                <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
                  Local distributed mode keeps first-run testing simple. HarborShield will save this plan now, then ask you to restart with the distributed Compose profile.
                </div>
              )}
            </div>
          ) : null}

          {error ? <div className="mt-4 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

          {nextAction ? (
            <div className="mt-5 rounded-3xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-900">
              <div className="text-sm font-semibold">{nextAction.title}</div>
              <p className="mt-2">{nextAction.body}</p>
              <pre className="mt-3 overflow-x-auto rounded-2xl bg-slate-950 px-4 py-3 text-xs text-slate-100">{nextAction.command}</pre>
            </div>
          ) : null}

          <div className="mt-6 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => void submit()}
              disabled={saving}
              className="rounded-2xl bg-ink px-5 py-3 text-sm font-medium text-white disabled:opacity-60"
            >
              {saving ? "Saving Setup..." : status.completed ? "Update Deployment Plan" : "Save Deployment Plan"}
            </button>
            <div className="text-sm text-slate-500">Signed in as {userEmail}</div>
          </div>
        </section>

        <aside className="space-y-4 rounded-[2rem] border border-slate-200 bg-white p-6 shadow-2xl">
          <div>
            <div className="text-xs uppercase tracking-[0.3em] text-slate-500">Runtime</div>
            <h2 className="mt-3 text-xl font-semibold text-ink">Current Stack</h2>
          </div>
          <FactRow label="Running Backend" value={status.runtimeStorageBackend === "distributed" ? "Distributed" : "Single node"} />
          <FactRow label="Runtime Endpoints" value={String(status.runtimeEndpointCount)} />
          <FactRow label="Planned Mode" value={planSummary} />
          <FactRow label="Status" value={status.applyRequired ? "Saved, apply required" : status.completed ? "Ready" : "Not saved yet"} />
          <FactRow label="Local Profile Command" value={status.recommendedLocalProfile} />
          <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
            HarborShield compares the saved deployment plan to the currently running stack. If they differ, the wizard stays here and shows the exact next step instead of silently dropping you into the dashboard.
          </div>
        </aside>
      </div>
    </div>
  );
}

function ChoiceCard({
  title,
  description,
  active,
  onClick,
  compact,
}: {
  title: string;
  description: string;
  active: boolean;
  onClick: () => void;
  compact?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-3xl border p-5 text-left transition ${
        active ? "border-orange-400 bg-orange-50" : "border-slate-200 bg-white hover:border-orange-300"
      } ${compact ? "" : "min-h-40"}`}
    >
      <div className="text-lg font-semibold text-ink">{title}</div>
      <p className="mt-2 text-sm text-slate-600">{description}</p>
    </button>
  );
}

function FactRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-slate-200 px-4 py-3">
      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{label}</div>
      <div className="mt-1 break-words text-sm text-ink">{value}</div>
    </div>
  );
}
