import { ArrowLeft, Database, Settings2, ShieldCheck, Wallet } from "lucide-react";

export const settingsRoutes = [
  { command: "/model", label: "Models", name: "Model settings", icon: Settings2 },
  { command: "/usage", label: "Usage & limits", name: "Usage & limits", icon: Wallet },
  { command: "/data", label: "Data & backups", name: "Data & backups", icon: Database },
  { command: "/recovery", label: "Recovery", name: "Recovery", icon: ShieldCheck },
] as const;

export function SettingsNavigation({ destination, collapsed, navigate }: { destination?: string; collapsed: boolean; navigate(command: string): void }) {
  return <div className="sidebar-scroll settings-navigation">
    <button className="nav-button settings-back" aria-label="Back to app" title={collapsed ? "Back to app" : undefined} onClick={() => navigate("/chat")}><ArrowLeft size={17} /><span>Back to app</span></button>
    <div className="nav-group-label">Settings</div>
    <nav aria-label="Settings categories">{settingsRoutes.map(({ command, label, name, icon: Icon }) => <button className="nav-button" key={command} aria-label={name} title={collapsed ? label : undefined} aria-current={destination === command ? "page" : undefined} onClick={() => navigate(command)}><Icon size={17} /><span>{label}</span></button>)}</nav>
  </div>;
}
