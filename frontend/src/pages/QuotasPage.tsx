import { useEffect, useState } from "react";
import { api } from "../api/client";
import { formatBytes, parseHumanBytes } from "../lib/format";

type BucketQuota = {
  bucketId: string;
  bucketName: string;
  currentBytes: number;
  currentObjects: number;
  maxBytes: number | null;
  maxObjects: number | null;
  warningThresholdPercent: number | null;
};

type UserQuota = {
  userId: string;
  email: string;
  currentBytes: number;
  maxBytes: number | null;
  warningThresholdPercent: number | null;
};

type QuotaResponse = {
  bucketItems: BucketQuota[];
  userItems: UserQuota[];
};

function normalizeQuotaResponse(value: Partial<QuotaResponse> | null | undefined): QuotaResponse {
  return {
    bucketItems: Array.isArray(value?.bucketItems) ? value.bucketItems : [],
    userItems: Array.isArray(value?.userItems) ? value.userItems : [],
  };
}

function toByteFieldValue(value: number | null) {
  return value === null ? "" : formatBytes(value);
}

function toIntegerFieldValue(value: number | null) {
  return value === null ? "" : String(value);
}

function formatLimitLabel(value: number | null) {
  return value === null ? "No limit configured" : formatBytes(value);
}

function formatObjectLimitLabel(value: number | null) {
  return value === null ? "No limit configured" : String(value);
}

function formatAlertThresholdLabel(value: number | null) {
  return value === null ? "No alert threshold" : `${value}%`;
}

function parseOptionalInt(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  const parsed = Number.parseInt(trimmed, 10);
  if (Number.isNaN(parsed)) {
    throw new Error("Quota values must be whole numbers");
  }
  return parsed;
}

function parseOptionalBytes(value: string) {
  return parseHumanBytes(value);
}

export function QuotasPage() {
  const [bucketItems, setBucketItems] = useState<BucketQuota[]>([]);
  const [userItems, setUserItems] = useState<UserQuota[]>([]);
  const [bucketDrafts, setBucketDrafts] = useState<Record<string, { maxBytes: string; maxObjects: string; warningThresholdPercent: string }>>({});
  const [userDrafts, setUserDrafts] = useState<Record<string, { maxBytes: string; warningThresholdPercent: string }>>({});
  const [error, setError] = useState("");
  const [savingKey, setSavingKey] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    const result = normalizeQuotaResponse(await api<QuotaResponse>("/quotas"));
    setBucketItems(result.bucketItems);
    setUserItems(result.userItems);
    setBucketDrafts(
      Object.fromEntries(
        result.bucketItems.map((item) => [
          item.bucketId,
          {
            maxBytes: toByteFieldValue(item.maxBytes),
            maxObjects: toIntegerFieldValue(item.maxObjects),
            warningThresholdPercent: toIntegerFieldValue(item.warningThresholdPercent),
          },
        ]),
      ),
    );
    setUserDrafts(
      Object.fromEntries(
        result.userItems.map((item) => [
          item.userId,
          {
            maxBytes: toByteFieldValue(item.maxBytes),
            warningThresholdPercent: toIntegerFieldValue(item.warningThresholdPercent),
          },
        ]),
      ),
    );
  }

  async function saveBucket(bucketId: string) {
    const draft = bucketDrafts[bucketId];
    if (!draft) {
      return;
    }
    setSavingKey(`bucket:${bucketId}`);
    setError("");
    try {
      await api(`/quotas/buckets/${bucketId}`, {
        method: "PUT",
        body: JSON.stringify({
          maxBytes: parseOptionalBytes(draft.maxBytes),
          maxObjects: parseOptionalInt(draft.maxObjects),
          warningThresholdPercent: parseOptionalInt(draft.warningThresholdPercent),
        }),
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update bucket quota");
    } finally {
      setSavingKey("");
    }
  }

  async function saveUser(userId: string) {
    const draft = userDrafts[userId];
    if (!draft) {
      return;
    }
    setSavingKey(`user:${userId}`);
    setError("");
    try {
      await api(`/quotas/users/${userId}`, {
        method: "PUT",
        body: JSON.stringify({
          maxBytes: parseOptionalBytes(draft.maxBytes),
          warningThresholdPercent: parseOptionalInt(draft.warningThresholdPercent),
        }),
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update user quota");
    } finally {
      setSavingKey("");
    }
  }

  useEffect(() => {
    setLoading(true);
    void load()
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load quotas"))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-3xl font-semibold text-ink">Quotas</h2>
        <p className="mt-1 max-w-2xl text-sm text-slate-600">
          Review current storage usage and set clear bucket or user limits using standard size units like MB, GB, and TB.
        </p>
      </div>

      {error ? <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</div> : null}

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">Bucket Storage Limits</h3>
            <p className="mt-1 text-sm text-slate-500">Set storage, object count, and alert thresholds for each bucket.</p>
          </div>
          <div className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{bucketItems.length} buckets</div>
        </div>

        <div className="mt-4 space-y-4">
          {loading ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Loading bucket quotas...</div>
          ) : bucketItems.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No buckets available yet.</div>
          ) : (
            bucketItems.map((item) => {
              const draft = bucketDrafts[item.bucketId] ?? { maxBytes: "", maxObjects: "", warningThresholdPercent: "" };
              const isSaving = savingKey === `bucket:${item.bucketId}`;
              return (
                <div key={item.bucketId} className="rounded-2xl border border-slate-200 px-4 py-4">
                  <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                    <div>
                      <div className="font-medium text-ink">{item.bucketName}</div>
                      <div className="mt-1 text-sm text-slate-500">{item.bucketId}</div>
                    </div>
                    <div className="grid gap-2 text-sm text-slate-600 md:grid-cols-2">
                      <div className="rounded-2xl bg-slate-50 px-3 py-2">Current storage: {formatBytes(item.currentBytes)}</div>
                      <div className="rounded-2xl bg-slate-50 px-3 py-2">
                        Current objects: {item.currentObjects}
                        {item.maxObjects !== null ? ` / ${item.maxObjects}` : ""}
                      </div>
                    </div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-500">
                    <div className="rounded-full bg-slate-100 px-3 py-1">
                      Storage limit: {formatLimitLabel(item.maxBytes)}
                    </div>
                    <div className="rounded-full bg-slate-100 px-3 py-1">
                      Object limit: {formatObjectLimitLabel(item.maxObjects)}
                    </div>
                    <div className="rounded-full bg-slate-100 px-3 py-1">
                      Alert threshold: {formatAlertThresholdLabel(item.warningThresholdPercent)}
                    </div>
                  </div>

                  <div className="mt-4 grid gap-3 lg:grid-cols-[1fr_1fr_1fr_auto]">
                    <input
                      className="rounded-2xl border border-slate-200 px-4 py-3"
                      placeholder="Storage limit, for example 500 MB"
                      value={draft.maxBytes}
                      onChange={(event) =>
                        setBucketDrafts((current) => ({
                          ...current,
                          [item.bucketId]: { ...draft, maxBytes: event.target.value },
                        }))
                      }
                    />
                    <input
                      className="rounded-2xl border border-slate-200 px-4 py-3"
                      placeholder="Object limit"
                      value={draft.maxObjects}
                      onChange={(event) =>
                        setBucketDrafts((current) => ({
                          ...current,
                          [item.bucketId]: { ...draft, maxObjects: event.target.value },
                        }))
                      }
                    />
                    <input
                      className="rounded-2xl border border-slate-200 px-4 py-3"
                      placeholder="Alert threshold, for example 80"
                      value={draft.warningThresholdPercent}
                      onChange={(event) =>
                        setBucketDrafts((current) => ({
                          ...current,
                          [item.bucketId]: { ...draft, warningThresholdPercent: event.target.value },
                        }))
                      }
                    />
                    <button
                      className="rounded-2xl bg-ink px-5 py-3 text-white disabled:opacity-60"
                      disabled={isSaving}
                      onClick={() => void saveBucket(item.bucketId)}
                      type="button"
                    >
                      {isSaving ? "Saving..." : "Save"}
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </section>

      <section className="rounded-3xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-ink">User Storage Limits</h3>
            <p className="mt-1 text-sm text-slate-500">Set storage limits and alert thresholds for platform users.</p>
          </div>
          <div className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{userItems.length} users</div>
        </div>

        <div className="mt-4 space-y-4">
          {loading ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">Loading user quotas...</div>
          ) : userItems.length === 0 ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-4 text-sm text-slate-500">No users available yet.</div>
          ) : (
            userItems.map((item) => {
              const draft = userDrafts[item.userId] ?? { maxBytes: "", warningThresholdPercent: "" };
              const isSaving = savingKey === `user:${item.userId}`;
              return (
                <div key={item.userId} className="rounded-2xl border border-slate-200 px-4 py-4">
                  <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                    <div>
                      <div className="font-medium text-ink">{item.email}</div>
                      <div className="mt-1 text-sm text-slate-500">{item.userId}</div>
                    </div>
                    <div className="rounded-2xl bg-slate-50 px-3 py-2 text-sm text-slate-600">Current storage: {formatBytes(item.currentBytes)}</div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-500">
                    <div className="rounded-full bg-slate-100 px-3 py-1">
                      Storage limit: {formatLimitLabel(item.maxBytes)}
                    </div>
                    <div className="rounded-full bg-slate-100 px-3 py-1">
                      Alert threshold: {formatAlertThresholdLabel(item.warningThresholdPercent)}
                    </div>
                  </div>

                  <div className="mt-4 grid gap-3 lg:grid-cols-[1fr_1fr_auto]">
                    <input
                      className="rounded-2xl border border-slate-200 px-4 py-3"
                      placeholder="Storage limit, for example 2 GB"
                      value={draft.maxBytes}
                      onChange={(event) =>
                        setUserDrafts((current) => ({
                          ...current,
                          [item.userId]: { ...draft, maxBytes: event.target.value },
                        }))
                      }
                    />
                    <input
                      className="rounded-2xl border border-slate-200 px-4 py-3"
                      placeholder="Alert threshold, for example 80"
                      value={draft.warningThresholdPercent}
                      onChange={(event) =>
                        setUserDrafts((current) => ({
                          ...current,
                          [item.userId]: { ...draft, warningThresholdPercent: event.target.value },
                        }))
                      }
                    />
                    <button
                      className="rounded-2xl bg-ink px-5 py-3 text-white disabled:opacity-60"
                      disabled={isSaving}
                      onClick={() => void saveUser(item.userId)}
                      type="button"
                    >
                      {isSaving ? "Saving..." : "Save"}
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </section>
    </div>
  );
}
