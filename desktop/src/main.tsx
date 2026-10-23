import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles.css";

// Restore the last rendered appearance before the first React paint. The
// backend's saved profile preference replaces this cache when it connects.
try { document.documentElement.dataset.theme = localStorage.getItem("gator:theme") || "gator"; } catch { /* Storage is optional. */ }

class ErrorBoundary extends React.Component<{ children: React.ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    return this.state.failed ? <main className="boot-screen"><h1>Gator’s view could not load.</h1><p>Your saved work is retained. Reload the window to reconnect.</p><button className="button" onClick={() => location.reload()}>Reload window</button></main> : this.props.children;
  }
}

createRoot(document.getElementById("root")!).render(<ErrorBoundary><App /></ErrorBoundary>);
