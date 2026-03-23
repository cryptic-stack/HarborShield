import { NavLink, Outlet } from "react-router-dom";

const navItems = [
  ["Dashboard", "/"],
  ["Buckets", "/buckets"],
  ["Objects", "/objects"],
  ["Uploads", "/uploads"],
  ["Users", "/users"],
  ["Credentials", "/credentials"],
  ["Roles", "/roles"],
  ["Policy Lab", "/policy-lab"],
  ["Audit", "/audit"],
  ["Events", "/events"],
  ["Malware", "/malware"],
  ["Quotas", "/quotas"],
  ["Health", "/health"],
  ["Storage", "/storage"],
  ["Settings", "/settings"],
];

type ShellProps = {
  userEmail: string;
  onLogout: () => void | Promise<void>;
};

export function Shell({ userEmail, onLogout }: ShellProps) {
  return (
    <div className="min-h-screen px-4 py-6 md:px-8">
      <div className="mx-auto grid max-w-7xl gap-6 md:grid-cols-[260px_1fr]">
        <aside
          className="rounded-3xl border border-slate-800 bg-slate-950/75 p-5 shadow-2xl shadow-black/30 backdrop-blur"
          style={{ backgroundColor: "rgba(2, 6, 23, 0.92)" }}
        >
          <div className="mb-6">
            <div className="text-xs uppercase tracking-[0.3em] text-orange-300">HarborShield</div>
            <h1 className="mt-2 text-2xl font-semibold text-slate-100">Object Platform</h1>
            <div className="mt-3 text-xs text-slate-400">{userEmail}</div>
          </div>
          <nav className="space-y-2">
            {navItems.map(([label, to]) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  `block rounded-2xl px-4 py-3 text-sm transition ${
                    isActive ? "bg-orange-500 text-slate-950" : "bg-slate-900 text-slate-300 hover:bg-slate-800"
                  }`
                }
              >
                {label}
              </NavLink>
            ))}
          </nav>
          <button
            type="button"
            onClick={() => {
              void onLogout();
            }}
            className="mt-6 w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-slate-300 transition hover:border-orange-400 hover:text-orange-200"
          >
            Sign Out
          </button>
        </aside>
        <main
          className="rounded-3xl border border-slate-800 bg-slate-950/70 p-6 shadow-2xl shadow-black/30 backdrop-blur"
          style={{ backgroundColor: "rgba(2, 6, 23, 0.88)" }}
        >
          <Outlet />
        </main>
      </div>
    </div>
  );
}
