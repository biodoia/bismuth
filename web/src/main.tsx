import React from "react";
import ReactDOM from "react-dom/client";
import App from "./pages/App";
// xterm.css moved into components/Terminal.tsx so it ships with the
// lazy terminal chunk instead of the entry bundle (P7-f).
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
