import { initializeApp } from "firebase/app";
import { getAuth, GoogleAuthProvider } from "firebase/auth";

const creds = {
  apiKey: import.meta.env.VITE_FB_API_KEY!,
  authDomain: import.meta.env.VITE_FB_AUTH_URI!,
  projectId: import.meta.env.VITE_FB_PROJECT_ID!,
}

const app = initializeApp(creds);

export const auth = getAuth(app);
export const googleProvider = new GoogleAuthProvider();
// Optional: force account picker each time
// googleProvider.setCustomParameters({ prompt: "select_account" });