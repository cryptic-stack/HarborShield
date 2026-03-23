import { useEffect, useState } from "react";
import { api } from "../api/client";

type User = { id: string; email: string; role: string; mustChangePassword: boolean };

export function UsersPage() {
  const [items, setItems] = useState<User[]>([]);

  useEffect(() => {
    void api<{ items: User[] }>("/users").then((result) => setItems(result.items ?? []));
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Users</h2>
        <p className="mt-1 text-sm text-slate-600">Platform auth identities stay separate from S3 service credentials.</p>
      </div>
      <div className="grid gap-3">
        {items.map((item) => (
          <div key={item.id} className="rounded-2xl border border-slate-200 px-4 py-4">
            <div className="font-medium">{item.email}</div>
            <div className="mt-1 text-sm text-slate-500">
              {item.role} {item.mustChangePassword ? "• password rotation required" : ""}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
