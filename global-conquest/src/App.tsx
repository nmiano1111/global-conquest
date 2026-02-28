import { RouterProvider } from "@tanstack/react-router";
import { useAuth } from "./auth";
import { router } from "./router";
import "./App.css";

function App() {
  const auth = useAuth();
  return <RouterProvider router={router} context={{ auth }} />;
}

export default App;
