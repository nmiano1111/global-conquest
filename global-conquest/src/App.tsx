import { useEffect, useState } from "react";
import { getHealth } from "./api/health";
import './App.css'

function App() {
  const [status, setStatus] = useState<string>("loading...");

  useEffect(() => {
    getHealth()
      .then((r) => setStatus(r.message))
      .catch((e) => setStatus(`error: ${e.status ?? "?"} ${e.message}`));
  }, []);

  return <div>{status}</div>;
}

export default App
