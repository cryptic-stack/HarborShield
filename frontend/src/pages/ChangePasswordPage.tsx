import { FormEvent, useState } from "react";

type Props = {
  email: string;
  onSubmit: (currentPassword: string, newPassword: string) => Promise<void>;
};

export function ChangePasswordPage({ email, onSubmit }: Props) {
  const [currentPassword, setCurrentPassword] = useState("change_me_now");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (newPassword.length < 12) {
      setError("New password must be at least 12 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("New passwords do not match");
      return;
    }

    try {
      setBusy(true);
      setError("");
      await onSubmit(currentPassword, newPassword);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to change password");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <form
        onSubmit={handleSubmit}
        className="auth-surface w-full max-w-md rounded-[2rem] border border-slate-200 p-8 shadow-2xl"
      >
        <div className="text-xs uppercase tracking-[0.35em] text-ember">HarborShield</div>
        <h1 className="mt-3 text-3xl font-semibold text-ink">Change Bootstrap Password</h1>
        <p className="mt-2 text-sm text-slate-600">
          The bootstrap admin for {email} must set a new password before using the platform.
        </p>
        <label className="auth-label mt-6 block text-sm">
          Current Password
          <input
            className="auth-input mt-2 w-full rounded-2xl px-4 py-3 text-slate-900"
            type="password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
          />
        </label>
        <label className="auth-label mt-4 block text-sm">
          New Password
          <input
            className="auth-input mt-2 w-full rounded-2xl px-4 py-3 text-slate-900"
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
          />
        </label>
        <label className="auth-label mt-4 block text-sm">
          Confirm New Password
          <input
            className="auth-input mt-2 w-full rounded-2xl px-4 py-3 text-slate-900"
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
        </label>
        {error ? <div className="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div> : null}
        <button className="mt-6 w-full rounded-2xl bg-ink px-4 py-3 text-sm font-medium text-white disabled:opacity-60" disabled={busy}>
          {busy ? "Updating..." : "Update Password"}
        </button>
      </form>
    </div>
  );
}
