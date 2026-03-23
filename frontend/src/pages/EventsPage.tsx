import { FormEvent, useEffect, useState } from "react";
import { api } from "../api/client";

type EventTarget = {
  id: string;
  name: string;
  targetType: string;
  endpointUrl: string;
  eventTypes: string[];
  enabled: boolean;
  createdAt: string;
};

type Delivery = {
  id: string;
  targetId: string;
  targetName: string;
  eventType: string;
  status: string;
  attempts: number;
  lastError: string;
  lastResponseCode?: number;
  createdAt: string;
  updatedAt: string;
};

export function EventsPage() {
  const [targets, setTargets] = useState<EventTarget[]>([]);
  const [deliveries, setDeliveries] = useState<Delivery[]>([]);
  const [statusFilter, setStatusFilter] = useState("");
  const [targetFilter, setTargetFilter] = useState("");
  const [eventTypeFilter, setEventTypeFilter] = useState("");
  const [name, setName] = useState("");
  const [endpointUrl, setEndpointUrl] = useState("");
  const [signingSecret, setSigningSecret] = useState("");
  const [eventTypes, setEventTypes] = useState("bucket.created,object.created");

  async function load() {
    const deliveryQuery = new URLSearchParams({ limit: "50" });
    if (statusFilter) {
      deliveryQuery.set("status", statusFilter);
    }
    if (targetFilter) {
      deliveryQuery.set("targetId", targetFilter);
    }
    if (eventTypeFilter) {
      deliveryQuery.set("eventType", eventTypeFilter);
    }

    const [targetResult, deliveryResult] = await Promise.all([
      api<{ items: EventTarget[] }>("/event-targets"),
      api<{ items: Delivery[] }>(`/event-deliveries?${deliveryQuery.toString()}`),
    ]);
    setTargets(targetResult.items ?? []);
    setDeliveries(deliveryResult.items ?? []);
  }

  useEffect(() => {
    void load();
  }, [statusFilter, targetFilter, eventTypeFilter]);

  async function createTarget(event: FormEvent) {
    event.preventDefault();
    await api("/event-targets", {
      method: "POST",
      body: JSON.stringify({
        name,
        endpointUrl,
        signingSecret,
        eventTypes: eventTypes
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
      }),
    });
    setName("");
    setEndpointUrl("");
    setSigningSecret("");
    setEventTypes("bucket.created,object.created");
    await load();
  }

  const deadLetterCount = deliveries.filter((delivery) => delivery.status === "dead_letter").length;
  const retryingCount = deliveries.filter((delivery) => delivery.status === "retrying" || delivery.status === "running").length;
  const deliveredCount = deliveries.filter((delivery) => delivery.status === "delivered").length;

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Events</h2>
        <p className="mt-1 text-sm text-slate-600">Manage webhook targets and inspect delivery outcomes, retries, and dead-letter state.</p>
      </div>

      <form onSubmit={createTarget} className="grid gap-4 rounded-3xl border border-slate-200 bg-slate-50 p-5 lg:grid-cols-2">
        <input className="rounded-2xl border border-slate-200 px-4 py-3" placeholder="Target name" value={name} onChange={(event) => setName(event.target.value)} />
        <input className="rounded-2xl border border-slate-200 px-4 py-3" placeholder="https://example.test/webhook" value={endpointUrl} onChange={(event) => setEndpointUrl(event.target.value)} />
        <input className="rounded-2xl border border-slate-200 px-4 py-3" placeholder="Optional signing secret" value={signingSecret} onChange={(event) => setSigningSecret(event.target.value)} />
        <input className="rounded-2xl border border-slate-200 px-4 py-3" placeholder="bucket.created,object.created,*" value={eventTypes} onChange={(event) => setEventTypes(event.target.value)} />
        <div className="lg:col-span-2">
          <button className="rounded-2xl bg-ink px-4 py-3 text-sm font-medium text-white transition hover:bg-slate-800" type="submit">
            Create webhook target
          </button>
        </div>
      </form>

      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-xl font-semibold text-ink">Targets</h3>
          <button className="rounded-2xl border border-slate-200 px-3 py-2 text-sm text-slate-700 transition hover:border-slate-300" onClick={() => void load()} type="button">
            Refresh
          </button>
        </div>
        <div className="grid gap-3">
          {targets.map((target) => (
            <div key={target.id} className="rounded-2xl border border-slate-200 px-4 py-4">
              <div className="flex flex-wrap items-center gap-3">
                <div className="font-medium text-ink">{target.name}</div>
                <span className="rounded-full bg-sky-100 px-2 py-1 text-xs font-medium text-sky-700">{target.targetType}</span>
                <span className={`rounded-full px-2 py-1 text-xs font-medium ${target.enabled ? "bg-emerald-100 text-emerald-700" : "bg-slate-100 text-slate-600"}`}>
                  {target.enabled ? "enabled" : "disabled"}
                </span>
              </div>
              <div className="mt-2 text-sm text-slate-600">{target.endpointUrl}</div>
              <div className="mt-2 text-xs text-slate-500">Events: {target.eventTypes.join(", ")}</div>
            </div>
          ))}
          {targets.length === 0 ? <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-6 text-sm text-slate-500">No webhook targets configured yet.</div> : null}
        </div>
      </section>

      <section className="space-y-4">
        <div className="grid gap-4 md:grid-cols-3">
          <SummaryCard label="Delivered" value={String(deliveredCount)} tone="emerald" />
          <SummaryCard label="Retrying" value={String(retryingCount)} tone="amber" />
          <SummaryCard label="Dead Letter" value={String(deadLetterCount)} tone="rose" />
        </div>

        <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h3 className="text-xl font-semibold text-ink">Recent Deliveries</h3>
            <p className="mt-1 text-sm text-slate-500">Filter delivery history by status, target, or event type to troubleshoot retries and dead-letter items faster.</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-3">
            <select className="rounded-2xl border border-slate-200 px-4 py-3 text-sm" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
              <option value="">All statuses</option>
              <option value="delivered">Delivered</option>
              <option value="retrying">Retrying</option>
              <option value="running">Running</option>
              <option value="dead_letter">Dead letter</option>
              <option value="pending">Pending</option>
            </select>
            <select className="rounded-2xl border border-slate-200 px-4 py-3 text-sm" value={targetFilter} onChange={(event) => setTargetFilter(event.target.value)}>
              <option value="">All targets</option>
              {targets.map((target) => (
                <option key={target.id} value={target.id}>
                  {target.name}
                </option>
              ))}
            </select>
            <input className="rounded-2xl border border-slate-200 px-4 py-3 text-sm" placeholder="object.created" value={eventTypeFilter} onChange={(event) => setEventTypeFilter(event.target.value)} />
          </div>
        </div>

        <div className="grid gap-3">
          {deliveries.map((delivery) => (
            <div key={delivery.id} className="rounded-2xl border border-slate-200 px-4 py-4">
              <div className="flex flex-wrap items-center gap-3">
                <div className="font-medium text-ink">{delivery.eventType}</div>
                <span className={`rounded-full px-2 py-1 text-xs font-medium ${
                  delivery.status === "delivered"
                    ? "bg-emerald-100 text-emerald-700"
                    : delivery.status === "dead_letter"
                    ? "bg-rose-100 text-rose-700"
                    : "bg-amber-100 text-amber-700"
                }`}>
                  {delivery.status}
                </span>
                <span className="text-xs text-slate-500">Attempts: {delivery.attempts}</span>
              </div>
              <div className="mt-2 text-sm text-slate-600">{delivery.targetName}</div>
              <div className="mt-1 text-xs text-slate-500">
                {new Date(delivery.createdAt).toLocaleString()}
                {delivery.lastResponseCode ? ` | HTTP ${delivery.lastResponseCode}` : ""}
                {delivery.updatedAt ? ` | updated ${new Date(delivery.updatedAt).toLocaleString()}` : ""}
              </div>
              {delivery.lastError ? <div className="mt-2 text-xs text-rose-600">{delivery.lastError}</div> : null}
            </div>
          ))}
          {deliveries.length === 0 ? <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-6 text-sm text-slate-500">No deliveries have been recorded yet.</div> : null}
        </div>
      </section>
    </div>
  );
}

function SummaryCard({ label, value, tone }: { label: string; value: string; tone: "emerald" | "amber" | "rose" }) {
  const toneClass =
    tone === "emerald"
      ? "border-emerald-200 bg-emerald-50 text-emerald-700"
      : tone === "amber"
      ? "border-amber-200 bg-amber-50 text-amber-700"
      : "border-rose-200 bg-rose-50 text-rose-700";

  return (
    <div className={`rounded-2xl border px-4 py-4 ${toneClass}`}>
      <div className="text-xs uppercase tracking-[0.2em]">{label}</div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
    </div>
  );
}
