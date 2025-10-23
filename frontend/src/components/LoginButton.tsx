import { signInWithPopup } from "firebase/auth";
import { auth, googleProvider } from "../lib/firebase";
import { useAuth } from "../state/auth";

export function LoginButtons() {
  const { user, loading, signOut } = useAuth();
  if (loading) return <div>Loading…</div>;
  if (user) {
    return (
      <div className="flex items-center gap-2">
        {user.photoURL && <img src={user.photoURL} width={24} height={24} style={{borderRadius:12}} />}
        <span>{user.displayName ?? user.email}</span>
        <button onClick={signOut}>Sign out</button>
      </div>
    );
  }
  return <button onClick={() => signInWithPopup(auth, googleProvider)}>Continue with Google</button>;
}
