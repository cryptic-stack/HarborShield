import { Navigate, Route, Routes } from "react-router-dom";
import { Shell } from "../components/Shell";
import { useSession } from "../hooks/useSession";
import { AuditPage } from "../pages/AuditPage";
import { BucketsPage } from "../pages/BucketsPage";
import { BucketDetailPage } from "../pages/BucketDetailPage";
import { ChangePasswordPage } from "../pages/ChangePasswordPage";
import { CredentialsPage } from "../pages/CredentialsPage";
import { DashboardPage } from "../pages/DashboardPage";
import { EventsPage } from "../pages/EventsPage";
import { HealthPage } from "../pages/HealthPage";
import { LoginPage } from "../pages/LoginPage";
import { MalwarePage } from "../pages/MalwarePage";
import { ObjectsPage } from "../pages/ObjectsPage";
import { PolicyLabPage } from "../pages/PolicyLabPage";
import { QuotasPage } from "../pages/QuotasPage";
import { RolesPage } from "../pages/RolesPage";
import { SettingsPage } from "../pages/SettingsPage";
import { SetupPage } from "../pages/SetupPage";
import { StoragePage } from "../pages/StoragePage";
import { UploadsPage } from "../pages/UploadsPage";
import { UsersPage } from "../pages/UsersPage";

export function App() {
  const { session, login, logout, changePassword, setupStatus, setupLoading, completeSetup } = useSession();

  if (!session) {
    return <LoginPage onLogin={login} />;
  }

  if (session.mustChangePassword) {
    return <ChangePasswordPage email={session.user.email} onSubmit={changePassword} />;
  }

  if (setupLoading || !setupStatus) {
    if (!setupLoading && !setupStatus) {
      return (
        <div className="flex min-h-screen items-center justify-center px-4">
          <div className="rounded-[2rem] border border-rose-200 bg-rose-50 p-8 text-sm text-rose-700 shadow-2xl">
            Unable to load deployment setup status. Refresh and try again.
          </div>
        </div>
      );
    }
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <div className="rounded-[2rem] border border-slate-200 bg-white p-8 text-sm text-slate-600 shadow-2xl">
          Loading deployment setup...
        </div>
      </div>
    );
  }

  if (setupStatus.required || setupStatus.applyRequired) {
    return <SetupPage userEmail={session.user.email} status={setupStatus} onSubmit={completeSetup} />;
  }

  return (
    <Routes>
      <Route element={<Shell userEmail={session.user.email} onLogout={logout} />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/buckets" element={<BucketsPage />} />
        <Route path="/buckets/:bucketId" element={<BucketDetailPage />} />
        <Route path="/objects" element={<ObjectsPage />} />
        <Route path="/uploads" element={<UploadsPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/credentials" element={<CredentialsPage />} />
        <Route path="/roles" element={<RolesPage />} />
        <Route path="/policy-lab" element={<PolicyLabPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/events" element={<EventsPage />} />
        <Route path="/malware" element={<MalwarePage />} />
        <Route path="/quotas" element={<QuotasPage />} />
        <Route path="/health" element={<HealthPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/storage" element={<StoragePage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
