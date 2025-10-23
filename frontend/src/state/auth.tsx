import React, {createContext, useContext, useEffect, useState} from "react";
import { auth } from "../lib/firebase";
import { onAuthStateChanged, signOut, type User } from "firebase/auth";

type AuthState = { user: User|null; idToken: string|null; loading: boolean; signOut: ()=>Promise<void> };
const Ctx = createContext<AuthState>({ user:null, idToken:null, loading:true, signOut: async()=>{} });

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User|null>(null);
  const [idToken, setIdToken] = useState<string|null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(()=> {
    const off = onAuthStateChanged(auth, async (u) => {
      setUser(u);
      setIdToken(u ? await u.getIdToken() : null);
      setLoading(false);
    });
    return off;
  }, []);

  return <Ctx.Provider value={{ user, idToken, loading, signOut: ()=>signOut(auth) }}>{children}</Ctx.Provider>;
}
export const useAuth = () => useContext(Ctx);
