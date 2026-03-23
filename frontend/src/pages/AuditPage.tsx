import { FormEvent, useEffect, useState } from "react";
import { api } from "../api/client";

type AuditItem = {
  actor: string;
  action: string;
  resource: string;
  outcome: string;
  category: string;
  severity: string;
  requestId: string;
  createdAt: string;
  detail?: Record<string, unknown>;
};

type Filters = {
  actor: string;
  action: string;
  outcome: string;
  category: string;
  severity: string;
  query: string;
};

type SettingsDiff = {
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  changedFields?: string[];
};

export function AuditPage() {
  const [items, setItems] = useState<AuditItem[]>([]);
  const [filters, setFilters] = useState<Filters>({
    actor: "",
    action: "",
    outcome: "",
    category: "",
    severity: "",
    query: "",
  });
  const [loading, setLoading] = useState(false);

  async function loadAudit(nextFilters: Filters) {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      Object.entries(nextFilters).forEach(([key, value]) => {
        if (value.trim()) {
          params.set(key, value.trim());
        }
      });
      params.set("limit", "200");
      const result = await api<{ items: AuditItem[] }>(`/audit?${params.toString()}`);
      setItems(result.items ?? []);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadAudit(filters);
  }, []);

  function updateFilter<K extends keyof Filters>(key: K, value: Filters[K]) {
    setFilters((current) => ({ ...current, [key]: value }));
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void loadAudit(filters);
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Audit Logs</h2>
        <p className="mt-1 text-sm text-slate-600">
          Search by actor, action, outcome, category, severity, or free text across resource, request ID, and detail payloads.
        </p>
      </div>
      <form onSubmit={handleSubmit} className="grid gap-3 rounded-3xl border border-slate-200 bg-white/80 p-4 md:grid-cols-3 xl:grid-cols-6">
        <input
          value={filters.actor}
          onChange={(event) => updateFilter("actor", event.target.value)}
          placeholder="Actor email"
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        />
        <input
          value={filters.action}
          onChange={(event) => updateFilter("action", event.target.value)}
          placeholder="Action"
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        />
        <select
          value={filters.outcome}
          onChange={(event) => updateFilter("outcome", event.target.value)}
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        >
          <option value="">All outcomes</option>
          <option value="success">Success</option>
          <option value="denied">Denied</option>
          <option value="failure">Failure</option>
        </select>
        <select
          value={filters.category}
          onChange={(event) => updateFilter("category", event.target.value)}
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        >
          <option value="">All categories</option>
          <option value="authentication">Authentication</option>
          <option value="settings">Settings</option>
          <option value="storage">Storage</option>
          <option value="data">Data</option>
          <option value="access-control">Access control</option>
          <option value="quota">Quota</option>
          <option value="malware">Malware</option>
          <option value="eventing">Eventing</option>
          <option value="deployment">Deployment</option>
          <option value="system">System</option>
        </select>
        <select
          value={filters.severity}
          onChange={(event) => updateFilter("severity", event.target.value)}
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        >
          <option value="">All severities</option>
          <option value="info">Info</option>
          <option value="low">Low</option>
          <option value="medium">Medium</option>
          <option value="high">High</option>
        </select>
        <input
          value={filters.query}
          onChange={(event) => updateFilter("query", event.target.value)}
          placeholder="Resource or request ID"
          className="rounded-2xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-amber-400"
        />
        <div className="flex items-center gap-3 md:col-span-3 xl:col-span-6">
          <button type="submit" className="rounded-2xl bg-ink px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800">
            {loading ? "Searching..." : "Search audit"}
          </button>
          <button
            type="button"
            className="rounded-2xl border border-slate-200 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-300"
            onClick={() => {
              const cleared = { actor: "", action: "", outcome: "", category: "", severity: "", query: "" };
              setFilters(cleared);
              void loadAudit(cleared);
            }}
          >
            Clear filters
          </button>
          <span className="text-sm text-slate-500">{items.length} matching records</span>
        </div>
      </form>
      <div className="grid gap-3">
        {items.map((item, index) => (
          <div key={`${item.createdAt}-${index}`} className="rounded-2xl border border-slate-200 px-4 py-4">
            <div className="flex flex-wrap items-center gap-3">
              <div className="font-medium text-ink">{item.action}</div>
              <Badge tone={outcomeTone(item.outcome)}>{item.outcome}</Badge>
              <Badge tone={categoryTone(item.category)}>{formatFieldLabel(item.category)}</Badge>
              <Badge tone={severityTone(item.severity)}>{formatFieldLabel(item.severity)}</Badge>
              <span className="text-xs text-slate-500">{new Date(item.createdAt).toLocaleString()}</span>
            </div>
            <div className="mt-2 text-sm text-slate-600">
              {item.actor} • {item.resource}
            </div>
            <div className="mt-1 text-xs text-slate-500">Request ID: {item.requestId || "n/a"}</div>
            {item.detail && Object.keys(item.detail).length > 0 ? <AuditDetail detail={item.detail} action={item.action} /> : null}
          </div>
        ))}
        {!loading && items.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-6 text-sm text-slate-500">
            No audit records matched the current filters.
          </div>
        ) : null}
      </div>
    </div>
  );
}

function Badge({ children, tone }: { children: string; tone: string }) {
  return <span className={`rounded-full px-2 py-1 text-xs font-medium ${tone}`}>{children}</span>;
}

function AuditDetail({ detail, action }: { detail: Record<string, unknown>; action: string }) {
  const diff = detail as SettingsDiff;
  const hasSettingsDiff = action.startsWith("settings.") && diff.before && diff.after;

  if (hasSettingsDiff) {
    const changedFields = Array.isArray(diff.changedFields) ? diff.changedFields : Object.keys(diff.after ?? {});
    return (
      <div className="mt-3 rounded-2xl bg-slate-50 px-3 py-3 text-xs text-slate-700">
        <div className="font-semibold text-slate-800">Changed fields</div>
        <div className="mt-2 flex flex-wrap gap-2">
          {changedFields.map((field) => (
            <span key={field} className="rounded-full bg-white px-2 py-1 text-[11px] font-medium text-slate-700">
              {formatFieldLabel(field)}
            </span>
          ))}
        </div>
        <div className="mt-3 grid gap-3 md:grid-cols-2">
          <div>
            <div className="mb-2 font-semibold text-slate-800">Before</div>
            <dl className="space-y-2">
              {changedFields.map((field) => (
                <div key={`before-${field}`} className="rounded-xl border border-slate-200 bg-white px-3 py-2">
                  <dt className="text-[11px] uppercase tracking-[0.15em] text-slate-500">{formatFieldLabel(field)}</dt>
                  <dd className="mt-1 break-words text-slate-700">{formatAuditValue(diff.before?.[field])}</dd>
                </div>
              ))}
            </dl>
          </div>
          <div>
            <div className="mb-2 font-semibold text-slate-800">After</div>
            <dl className="space-y-2">
              {changedFields.map((field) => (
                <div key={`after-${field}`} className="rounded-xl border border-slate-200 bg-white px-3 py-2">
                  <dt className="text-[11px] uppercase tracking-[0.15em] text-slate-500">{formatFieldLabel(field)}</dt>
                  <dd className="mt-1 break-words text-slate-700">{formatAuditValue(diff.after?.[field])}</dd>
                </div>
              ))}
            </dl>
          </div>
        </div>
      </div>
    );
  }

  return (
    <pre className="mt-3 overflow-x-auto rounded-2xl bg-slate-50 px-3 py-3 text-xs text-slate-600">{JSON.stringify(detail, null, 2)}</pre>
  );
}

function formatAuditValue(value: unknown): string {
  if (value === null || value === undefined || value === "") {
    return "Not set";
  }
  if (Array.isArray(value)) {
    return value.length ? value.map((item) => formatAuditValue(item)).join(", ") : "Not set";
  }
  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) {
      return "Not set";
    }
    return entries.map(([key, item]) => `${key}: ${formatAuditValue(item)}`).join(", ");
  }
  if (typeof value === "boolean") {
    return value ? "Enabled" : "Disabled";
  }
  return String(value);
}

function formatFieldLabel(value: string): string {
  return value
    .replace(/([A-Z])/g, " $1")
    .replace(/[-_]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/^\w/, (char) => char.toUpperCase());
}

function outcomeTone(value: string): string {
  switch (value) {
    case "success":
      return "bg-emerald-100 text-emerald-700";
    case "denied":
      return "bg-amber-100 text-amber-700";
    case "failure":
      return "bg-rose-100 text-rose-700";
    default:
      return "bg-slate-100 text-slate-700";
  }
}

function severityTone(value: string): string {
  switch (value) {
    case "high":
      return "bg-rose-100 text-rose-700";
    case "medium":
      return "bg-amber-100 text-amber-700";
    case "low":
      return "bg-sky-100 text-sky-700";
    default:
      return "bg-slate-100 text-slate-700";
  }
}

function categoryTone(value: string): string {
  switch (value) {
    case "authentication":
      return "bg-violet-100 text-violet-700";
    case "settings":
      return "bg-indigo-100 text-indigo-700";
    case "storage":
      return "bg-cyan-100 text-cyan-700";
    case "data":
      return "bg-blue-100 text-blue-700";
    case "access-control":
      return "bg-fuchsia-100 text-fuchsia-700";
    case "quota":
      return "bg-teal-100 text-teal-700";
    case "malware":
      return "bg-lime-100 text-lime-700";
    case "eventing":
      return "bg-orange-100 text-orange-700";
    case "deployment":
      return "bg-amber-100 text-amber-700";
    default:
      return "bg-slate-100 text-slate-700";
  }
}
