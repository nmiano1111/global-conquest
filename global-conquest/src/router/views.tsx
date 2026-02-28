import { useState } from "react";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../api/client";
import { login, signup } from "../api/auth";
import { useAuth } from "../auth";

export function RootLayout() {
  return <Outlet />;
}

export function LoginPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({ username: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!res.token || !res.user.id || !res.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(res.token, {
        id: res.user.id,
        username: res.user.username,
        role: res.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Login failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="page">
      <h1>Login</h1>
      <form className="auth-form" onSubmit={onSubmit}>
        <label>
          Username
          <input
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete="current-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>
        {error ? <p className="form-error">{error}</p> : null}
        <button type="submit" disabled={submitting}>
          {submitting ? "Signing in..." : "Login"}
        </button>
      </form>
      <p>
        Need an account? <Link to="/signup">Sign up</Link>
      </p>
    </main>
  );
}

export function SignupPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [form, setForm] = useState({
    username: "",
    password: "",
    confirmPassword: "",
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>("");

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError("");
    if (form.password !== form.confirmPassword) {
      setError("Passwords do not match");
      return;
    }

    setSubmitting(true);
    try {
      await signup({
        username: form.username.trim(),
        password: form.password,
      });
      const loginRes = await login({
        username: form.username.trim(),
        password: form.password,
      });
      if (!loginRes.token || !loginRes.user.id || !loginRes.user.username) {
        throw new Error("invalid login response from server");
      }
      auth.setSession(loginRes.token, {
        id: loginRes.user.id,
        username: loginRes.user.username,
        role: loginRes.user.role,
      });
      await navigate({ to: "/app/lobby" });
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.message || "Signup failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="page">
      <h1>Sign Up</h1>
      <form className="auth-form" onSubmit={onSubmit}>
        <label>
          Username
          <input
            type="text"
            autoComplete="username"
            minLength={3}
            maxLength={24}
            value={form.username}
            onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.password}
            onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
            required
          />
        </label>
        <label>
          Confirm Password
          <input
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={128}
            value={form.confirmPassword}
            onChange={(e) => setForm((prev) => ({ ...prev, confirmPassword: e.target.value }))}
            required
          />
        </label>
        {error ? <p className="form-error">{error}</p> : null}
        <button type="submit" disabled={submitting}>
          {submitting ? "Creating account..." : "Create account"}
        </button>
      </form>
      <p>
        Already have an account? <Link to="/login">Login</Link>
      </p>
    </main>
  );
}

export function AppShell() {
  const navigate = useNavigate();
  const auth = useAuth();

  const onLogout = async () => {
    auth.clearSession();
    await navigate({ to: "/login" });
  };

  return (
    <main className="page">
      <header className="app-header">
        <h1>Global Conquest</h1>
        <nav className="app-nav">
          <Link to="/app/lobby">Lobby</Link>
          <Link to="/app/profile">Profile</Link>
          <button type="button" onClick={onLogout}>
            Logout
          </button>
        </nav>
      </header>
      <section>
        <Outlet />
      </section>
    </main>
  );
}

export function LobbyPage() {
  const auth = useAuth();

  return (
    <div>
      <h2>Lobby</h2>
      <p>Welcome back, {auth.user?.username}.</p>
    </div>
  );
}

export function ProfilePage() {
  const auth = useAuth();

  return (
    <div>
      <h2>Profile</h2>
      <p>Username: {auth.user?.username ?? "-"}</p>
      <p>User ID: {auth.user?.id ?? "-"}</p>
      <p>Role: {auth.user?.role ?? "-"}</p>
    </div>
  );
}
