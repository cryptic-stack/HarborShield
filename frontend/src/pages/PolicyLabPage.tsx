import { FormEvent, useState } from "react";
import { api } from "../api/client";

type Binding = {
  id: string;
  subjectType: string;
  subjectId: string;
  resource: string;
  roleName: string;
  createdAt: string;
};

type Statement = {
  id: string;
  roleName: string;
  action: string;
  resource: string;
  effect: string;
};

type Trace = {
  role: string;
  allowed: boolean;
  explicitDeny: boolean;
  matchedScopes?: Binding[];
  statements: Statement[];
};

type EvaluationResult = {
  subjectType: string;
  subjectId: string;
  action: string;
  resource: string;
  fallbackRole: string;
  effectiveRole: string;
  allowed: boolean;
  reason: string;
  bindings: Binding[];
  traces: Trace[];
};

const presets = [
  {
    label: "Scoped S3 PUT",
    subjectType: "credential",
    fallbackRole: "readonly",
    action: "object.put",
    resource: "bucket:demo-bucket/object:report.txt",
  },
  {
    label: "Bucket Delete",
    subjectType: "admin_token",
    fallbackRole: "readonly",
    action: "bucket.delete",
    resource: "bucket:archive",
  },
];

export function PolicyLabPage() {
  const [subjectType, setSubjectType] = useState("credential");
  const [subjectId, setSubjectId] = useState("");
  const [fallbackRole, setFallbackRole] = useState("readonly");
  const [action, setAction] = useState("object.put");
  const [resource, setResource] = useState("bucket:demo-bucket/object:report.txt");
  const [result, setResult] = useState<EvaluationResult | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function evaluate(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const payload = await api<EvaluationResult>("/policy-evaluate", {
        method: "POST",
        body: JSON.stringify({ subjectType, subjectId, fallbackRole, action, resource }),
      });
      setResult(payload);
    } catch (err) {
      setResult(null);
      setError(err instanceof Error ? err.message : "Policy evaluation failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h2 className="text-3xl font-semibold text-ink">Policy Lab</h2>
          <p className="mt-1 max-w-2xl text-sm text-slate-600">
            Preview a subject&apos;s effective role, matching bindings, and the statements that drive an allow or deny decision.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => (
            <button
              key={preset.label}
              className="rounded-2xl border border-slate-200 px-4 py-2 text-sm text-slate-700 hover:bg-slate-100"
              onClick={() => {
                setSubjectType(preset.subjectType);
                setFallbackRole(preset.fallbackRole);
                setAction(preset.action);
                setResource(preset.resource);
              }}
              type="button"
            >
              {preset.label}
            </button>
          ))}
        </div>
      </div>

      <form className="grid gap-4 rounded-3xl border border-slate-200 bg-slate-50 p-5 lg:grid-cols-2" onSubmit={(event) => void evaluate(event)}>
        <label className="space-y-2 text-sm text-slate-700">
          <span>Subject Type</span>
          <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={subjectType} onChange={(e) => setSubjectType(e.target.value)}>
            <option value="credential">credential</option>
            <option value="admin_token">admin_token</option>
            <option value="user">user</option>
          </select>
        </label>
        <label className="space-y-2 text-sm text-slate-700">
          <span>Fallback Role</span>
          <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={fallbackRole} onChange={(e) => setFallbackRole(e.target.value)} />
        </label>
        <label className="space-y-2 text-sm text-slate-700 lg:col-span-2">
          <span>Subject ID</span>
          <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="access key, token id, or user id" value={subjectId} onChange={(e) => setSubjectId(e.target.value)} />
        </label>
        <label className="space-y-2 text-sm text-slate-700">
          <span>Action</span>
          <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={action} onChange={(e) => setAction(e.target.value)} />
        </label>
        <label className="space-y-2 text-sm text-slate-700">
          <span>Resource</span>
          <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={resource} onChange={(e) => setResource(e.target.value)} />
        </label>
        <div className="lg:col-span-2">
          <button className="rounded-2xl bg-ink px-5 py-3 text-white disabled:opacity-60" disabled={loading || !subjectId.trim()} type="submit">
            {loading ? "Evaluating..." : "Evaluate"}
          </button>
        </div>
      </form>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      {result ? (
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-4">
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Decision</div>
              <div className={`mt-2 text-2xl font-semibold ${result.allowed ? "text-moss" : "text-rose-600"}`}>{result.allowed ? "Allow" : "Deny"}</div>
            </div>
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Effective Role</div>
              <div className="mt-2 text-lg font-semibold text-ink">{result.effectiveRole}</div>
            </div>
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Reason</div>
              <div className="mt-2 text-lg font-semibold text-ink">{result.reason}</div>
            </div>
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-slate-500">Matched Bindings</div>
              <div className="mt-2 text-lg font-semibold text-ink">{result.bindings.length}</div>
            </div>
          </div>

          <div className="rounded-3xl border border-slate-200 p-5">
            <h3 className="text-lg font-semibold text-ink">Matched Bindings</h3>
            <div className="mt-4 space-y-3">
              {result.bindings.length === 0 ? (
                <div className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-500">No scoped bindings matched this resource. The fallback role drove the decision.</div>
              ) : (
                result.bindings.map((binding) => (
                  <div key={binding.id} className="rounded-2xl bg-slate-50 px-4 py-4">
                    <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                      <div className="font-medium text-ink">{binding.roleName}</div>
                      <div className="text-xs uppercase tracking-[0.2em] text-slate-500">{binding.subjectType}</div>
                    </div>
                    <div className="mt-2 text-sm text-slate-600">{binding.resource}</div>
                    <div className="mt-1 text-xs text-slate-400">{binding.subjectId}</div>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="space-y-4">
            {result.traces.map((trace) => (
              <div key={trace.role} className="rounded-3xl border border-slate-200 p-5">
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div>
                    <h3 className="text-lg font-semibold text-ink">{trace.role}</h3>
                    <p className="text-sm text-slate-500">{trace.allowed ? "This role would allow the request." : trace.explicitDeny ? "This role explicitly denies the request." : "This role does not allow the request."}</p>
                  </div>
                  <span className={`rounded-full px-3 py-1 text-xs font-medium ${trace.allowed ? "bg-moss/10 text-moss" : "bg-rose-100 text-rose-600"}`}>
                    {trace.allowed ? "Allow" : trace.explicitDeny ? "Explicit Deny" : "No Match"}
                  </span>
                </div>

                {trace.matchedScopes && trace.matchedScopes.length > 0 ? (
                  <div className="mt-4 rounded-2xl bg-amber-50 px-4 py-3 text-sm text-amber-800">
                    Bound via {trace.matchedScopes.map((scope) => scope.resource).join(", ")}
                  </div>
                ) : null}

                <div className="mt-4 space-y-3">
                  {trace.statements.length === 0 ? (
                    <div className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-500">No statements matched this action and resource.</div>
                  ) : (
                    trace.statements.map((statement) => (
                      <div key={statement.id} className="rounded-2xl bg-slate-50 px-4 py-4">
                        <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                          <div className="font-medium text-ink">{statement.action}</div>
                          <span className={`rounded-full px-3 py-1 text-xs font-medium ${statement.effect === "allow" ? "bg-moss/10 text-moss" : "bg-rose-100 text-rose-600"}`}>
                            {statement.effect}
                          </span>
                        </div>
                        <div className="mt-2 text-sm text-slate-600">{statement.resource}</div>
                      </div>
                    ))
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
