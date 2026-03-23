import { useEffect, useState } from "react";
import { api } from "../api/client";

type Statement = {
  id: string;
  roleName: string;
  action: string;
  resource: string;
  effect: string;
};

type Role = {
  name: string;
  description: string;
  builtin: boolean;
  statements: Statement[];
};

type Binding = {
  id: string;
  subjectType: string;
  subjectId: string;
  resource: string;
  roleName: string;
  createdAt: string;
};

export function RolesPage() {
  const [roles, setRoles] = useState<Role[]>([]);
  const [bindings, setBindings] = useState<Binding[]>([]);
  const [selectedRole, setSelectedRole] = useState("");
  const [action, setAction] = useState("");
  const [resource, setResource] = useState("*");
  const [effect, setEffect] = useState("allow");
  const [subjectType, setSubjectType] = useState("credential");
  const [subjectId, setSubjectId] = useState("");
  const [bindingResource, setBindingResource] = useState("*");
  const [bindingRole, setBindingRole] = useState("");
  const [error, setError] = useState("");

  async function load() {
    const [roleResult, bindingResult] = await Promise.all([
      api<{ items: Role[] }>("/roles"),
      api<{ items: Binding[] }>("/role-bindings"),
    ]);
    const roleItems = roleResult.items ?? [];
    const bindingItems = bindingResult.items ?? [];
    setRoles(roleItems);
    setBindings(bindingItems);
    if (!selectedRole && roleItems[0]) {
      setSelectedRole(roleItems[0].name);
      setBindingRole(roleItems[0].name);
    }
  }

  async function addStatement() {
    setError("");
    try {
      await api(`/roles/${selectedRole}/statements`, {
        method: "POST",
        body: JSON.stringify({ action, resource, effect, conditions: {} }),
      });
      setAction("");
      setResource("*");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to add statement");
    }
  }

  async function addBinding() {
    setError("");
    try {
      await api("/role-bindings", {
        method: "POST",
        body: JSON.stringify({
          subjectType,
          subjectId,
          resource: bindingResource,
          roleName: bindingRole || selectedRole,
        }),
      });
      setSubjectId("");
      setBindingResource("*");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to add binding");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!bindingRole && selectedRole) {
      setBindingRole(selectedRole);
    }
  }, [bindingRole, selectedRole]);

  const currentRole = roles.find((role) => role.name === selectedRole);
  const scopedBindings = bindings.filter((binding) => binding.roleName === (bindingRole || selectedRole));

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Roles & Policies</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Review built-in roles, add policy statements, and bind roles to credentials, users, or admin tokens with optional resource scopes.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      <div className="grid gap-6 xl:grid-cols-[300px_1fr]">
        <aside className="space-y-3">
          {roles.map((role) => (
            <button
              key={role.name}
              className={`w-full rounded-2xl border px-4 py-4 text-left ${selectedRole === role.name ? "border-ink bg-ink text-white" : "border-slate-200 bg-white text-slate-700 hover:bg-slate-50"}`}
              onClick={() => {
                setSelectedRole(role.name);
                setBindingRole(role.name);
              }}
              type="button"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="font-medium">{role.name}</div>
                <span className={`rounded-full px-2 py-1 text-xs ${selectedRole === role.name ? "bg-white/20 text-white" : "bg-slate-100 text-slate-500"}`}>
                  {role.statements.length} rules
                </span>
              </div>
              <div className={`mt-2 text-sm ${selectedRole === role.name ? "text-white/80" : "text-slate-500"}`}>{role.description}</div>
            </button>
          ))}
        </aside>

        <div className="space-y-6">
          {currentRole ? (
            <>
              <section className="rounded-3xl border border-slate-200 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="text-lg font-semibold text-ink">{currentRole.name}</h3>
                    <p className="mt-1 text-sm text-slate-500">{currentRole.description}</p>
                  </div>
                  <span className="rounded-full bg-slate-100 px-3 py-1 text-xs uppercase tracking-[0.2em] text-slate-600">{currentRole.builtin ? "built-in" : "custom"}</span>
                </div>

                <div className="mt-5 grid gap-4 lg:grid-cols-[1fr_320px]">
                  <div className="space-y-3">
                    {currentRole.statements.map((statement) => (
                      <div key={statement.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                        <div className="flex items-center justify-between gap-3">
                          <div className="font-medium text-ink">{statement.action}</div>
                          <span className={`rounded-full px-3 py-1 text-xs font-medium ${statement.effect === "allow" ? "bg-moss/10 text-moss" : "bg-rose-100 text-rose-600"}`}>
                            {statement.effect}
                          </span>
                        </div>
                        <div className="mt-2 text-sm text-slate-500">{statement.resource}</div>
                      </div>
                    ))}
                  </div>

                  <div className="rounded-2xl bg-slate-50 p-4">
                    <h4 className="font-medium text-ink">Add Statement</h4>
                    <div className="mt-4 space-y-3">
                      <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="object.put" value={action} onChange={(event) => setAction(event.target.value)} />
                      <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="bucket:demo*" value={resource} onChange={(event) => setResource(event.target.value)} />
                      <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={effect} onChange={(event) => setEffect(event.target.value)}>
                        <option value="allow">allow</option>
                        <option value="deny">deny</option>
                      </select>
                      <button className="rounded-2xl bg-ink px-4 py-3 text-white" onClick={() => void addStatement()} type="button">
                        Add Statement
                      </button>
                    </div>
                  </div>
                </div>
              </section>

              <section className="rounded-3xl border border-slate-200 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="text-lg font-semibold text-ink">Role Bindings</h3>
                    <p className="mt-1 text-sm text-slate-500">Attach roles to specific subjects and resource scopes.</p>
                  </div>
                  <span className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{scopedBindings.length} bindings for {bindingRole || selectedRole}</span>
                </div>

                <div className="mt-5 grid gap-4 lg:grid-cols-[1fr_340px]">
                  <div className="space-y-3">
                    {scopedBindings.length === 0 ? (
                      <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No bindings exist for this role yet.</div>
                    ) : (
                      scopedBindings.map((binding) => (
                        <div key={binding.id} className="rounded-2xl border border-slate-200 px-4 py-4">
                          <div className="flex items-center justify-between gap-3">
                            <div className="font-medium text-ink">{binding.subjectId}</div>
                            <span className="rounded-full bg-slate-100 px-3 py-1 text-xs uppercase tracking-[0.2em] text-slate-600">{binding.subjectType}</span>
                          </div>
                          <div className="mt-2 text-sm text-slate-500">{binding.resource}</div>
                          <div className="mt-1 text-xs text-slate-400">{binding.createdAt}</div>
                        </div>
                      ))
                    )}
                  </div>

                  <div className="rounded-2xl bg-slate-50 p-4">
                    <h4 className="font-medium text-ink">Bind Subject</h4>
                    <div className="mt-4 space-y-3">
                      <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={subjectType} onChange={(event) => setSubjectType(event.target.value)}>
                        <option value="credential">credential</option>
                        <option value="admin_token">admin_token</option>
                        <option value="user">user</option>
                      </select>
                      <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="subject id" value={subjectId} onChange={(event) => setSubjectId(event.target.value)} />
                      <input className="w-full rounded-2xl border border-slate-200 px-4 py-3" placeholder="*" value={bindingResource} onChange={(event) => setBindingResource(event.target.value)} />
                      <select className="w-full rounded-2xl border border-slate-200 px-4 py-3" value={bindingRole || selectedRole} onChange={(event) => setBindingRole(event.target.value)}>
                        {roles.map((role) => (
                          <option key={role.name} value={role.name}>
                            {role.name}
                          </option>
                        ))}
                      </select>
                      <button className="rounded-2xl bg-ink px-4 py-3 text-white" onClick={() => void addBinding()} type="button">
                        Create Binding
                      </button>
                    </div>
                  </div>
                </div>
              </section>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}
