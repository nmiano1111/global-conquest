// src/components/EmailAuth.tsx
import { useState } from "react";
import { signupEmailPassword, loginEmailPassword, resetPassword } from "../lib/emailAuth";
import { useAuth } from "../state/auth";

export function EmailAuth() {
  const { user } = useAuth();
  const [mode, setMode] = useState<"login"|"signup">("login");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [info, setInfo] = useState<string | null>(null);

  if (user) return null;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null); setInfo(null); setBusy(true);
    try {
      if (mode === "signup") {
        await signupEmailPassword(email.trim(), password, displayName.trim() || undefined);
        setInfo("Account created. You are signed in.");
      } else {
        await loginEmailPassword(email.trim(), password);
      }
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onReset() {
    setErr(null); setInfo(null); setBusy(true);
    try {
      await resetPassword(email.trim());
      setInfo("Password reset email sent (if the address exists).");
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} style={{ display: "grid", gap: 8 }}>
      <div style={{ display: "flex", gap: 8 }}>
        <button type="button" onClick={() => setMode("login")} disabled={mode==="login"}>Log in</button>
        <button type="button" onClick={() => setMode("signup")} disabled={mode==="signup"}>Sign up</button>
      </div>

      {mode === "signup" && (
        <input placeholder="Display name (optional)" value={displayName} onChange={e=>setDisplayName(e.target.value)} />
      )}
      <input type="email" placeholder="email" value={email} onChange={e=>setEmail(e.target.value)} required />
      <input type="password" placeholder="password" value={password} onChange={e=>setPassword(e.target.value)} required />

      <button type="submit" disabled={busy}>
        {mode === "signup" ? "Create account" : "Log in"}
      </button>

      <button type="button" onClick={onReset} disabled={!email || busy} style={{ justifySelf: "start" }}>
        Forgot password
      </button>

      {err && <div style={{ color: "crimson" }}>{err}</div>}
      {info && <div style={{ color: "seagreen" }}>{info}</div>}
    </form>
  );
}
