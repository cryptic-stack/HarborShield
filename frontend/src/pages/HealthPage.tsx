import { useEffect, useState } from "react";
import { api } from "../api/client";

type HealthSnapshot = {
  admin: string;
  healthz: string;
  readyz: string;
  metricsPreview: string[];
};

async function fetchText(path: string) {
  const response = await fetch(path);
  if (!response.ok) {
    return `${response.status} ${response.statusText}`;
  }
  return response.text();
}

export function HealthPage() {
  const [snapshot, setSnapshot] = useState<HealthSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const [admin, healthz, readyz, metrics] = await Promise.all([
        api<{ status: string }>("/health"),
        fetchText("/healthz"),
        fetchText("/readyz"),
        fetchText("/metrics"),
      ]);
      setSnapshot({
        admin: admin.status,
        healthz,
        readyz,
        metricsPreview: metrics.split("\n").filter((line) => line && !line.startsWith("#")).slice(0, 8),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load health data");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-3xl font-semibold text-ink">System Health</h2>
          <p className="mt-1 max-w-2xl text-sm text-slate-600">
            Track admin-plane health, readiness, and a quick live preview of exported Prometheus metrics from the running stack.
          </p>
        </div>
        <button className="rounded-2xl bg-ink px-4 py-3 text-white" onClick={() => void load()} type="button">
          Refresh
        </button>
      </div>

      {loading ? <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Loading health status...</div> : null}
      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      {snapshot ? (
        <>
          <div className="grid gap-4 md:grid-cols-3">
            <div className="rounded-3xl border border-slate-200 p-5">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Admin API</div>
              <div className="mt-2 text-2xl font-semibold text-moss">{snapshot.admin}</div>
            </div>
            <div className="rounded-3xl border border-slate-200 p-5">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Healthz</div>
              <pre className="mt-2 whitespace-pre-wrap text-sm text-slate-700">{snapshot.healthz}</pre>
            </div>
            <div className="rounded-3xl border border-slate-200 p-5">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Readyz</div>
              <pre className="mt-2 whitespace-pre-wrap text-sm text-slate-700">{snapshot.readyz}</pre>
            </div>
          </div>

          <div className="rounded-3xl border border-slate-200 p-5">
            <h3 className="text-lg font-semibold text-ink">Metrics Preview</h3>
            <p className="mt-1 text-sm text-slate-500">A quick sample of live Prometheus-exported values from the rebuilt stack.</p>
            <div className="mt-4 space-y-2 rounded-2xl bg-slate-950 px-4 py-4 font-mono text-sm text-emerald-200">
              {snapshot.metricsPreview.map((line) => (
                <div key={line}>{line}</div>
              ))}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}
