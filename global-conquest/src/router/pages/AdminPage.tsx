import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { listAdminUsers, revokeUserSessions, updateUserAccess, type AdminUserRecord } from "../../api/users";
import { useAuth } from "../../auth";
import { buttonGhostClass, buttonPrimaryClass } from "./styles";

export function AdminPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [users, setUsers] = useState<AdminUserRecord[]>([]);
  const [busyUserID, setBusyUserID] = useState("");

  const load = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await listAdminUsers();
        if (cancelled) return;
        setUsers(out);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        if (apiErr.status === 403) {
          await navigate({ to: "/app/lobby" });
          return;
        }
        setError(apiErr.message || "Failed to load admin users");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await load(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [load]);

  const patchAccess = async (u: AdminUserRecord, accessStatus: "active" | "blocked") => {
    setBusyUserID(u.id);
    setError("");
    try {
      await updateUserAccess(u.id, accessStatus);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to update user access");
    } finally {
      setBusyUserID("");
    }
  };

  const onRevokeSessions = async (u: AdminUserRecord) => {
    setBusyUserID(u.id);
    setError("");
    try {
      await revokeUserSessions(u.id);
      await load();
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      if (apiErr.status === 403) {
        await navigate({ to: "/app/lobby" });
        return;
      }
      setError(apiErr.message || "Failed to revoke sessions");
    } finally {
      setBusyUserID("");
    }
  };

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Admin Dashboard</h2>
          <p className="text-sm text-slate-600">Manage user access and active sessions.</p>
        </div>
        <button className={buttonGhostClass} type="button" onClick={() => void load()} disabled={loading || !!busyUserID}>
          Refresh
        </button>
      </div>

      {loading ? <p className="text-sm text-slate-600">Loading users...</p> : null}
      {error ? <p className="mb-3 text-sm text-rose-700">{error}</p> : null}

      <div className="overflow-x-auto rounded-xl border border-slate-200">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">User</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Role</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Access</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Sessions</th>
              <th className="px-3 py-2 text-left font-semibold text-slate-700">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {users.map((u) => {
              const isBusy = busyUserID === u.id;
              const isBlocked = u.accessStatus === "blocked";
              const accessLabel = isBlocked ? "Blocked" : "Active";
              return (
                <tr key={u.id}>
                  <td className="px-3 py-2">
                    <p className="font-medium text-slate-900">{u.username}</p>
                    <p className="font-mono text-[11px] text-slate-500">{u.id}</p>
                  </td>
                  <td className="px-3 py-2 text-slate-700">{u.role}</td>
                  <td className="px-3 py-2 text-slate-700">{accessLabel}</td>
                  <td className="px-3 py-2 text-slate-700">{u.activeSessions}</td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-2">
                      {isBlocked ? (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "active")}
                        >
                          Unblock
                        </button>
                      ) : (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "blocked")}
                        >
                          Block
                        </button>
                      )}
                      <button className={buttonPrimaryClass} type="button" disabled={isBusy} onClick={() => void onRevokeSessions(u)}>
                        Revoke Sessions
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
