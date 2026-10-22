import { ArrowLeft, Database, Info, Plug, Settings2, ShieldCheck, Wallet } from "lucide-react";

export const settingsRoutes = [
  { command: "/connections", label: "Connections", name: "Connection settings", icon: Plug },
  { command: "/model", label: "Models", name: "Model settings", icon: Settings2 },
  { command: "/usage", label: "Usage & limits", name: "Usage & limits", icon: Wallet },
  { command: "/data", label: "Data & backups", name: "Data & backups", icon: Database },
  { command: "/recovery", label: "Recovery", name: "Recovery", icon: ShieldCheck },
] as const;

export function SettingsNavigation({ destination, collapsed, navigate, about }: { destination?: string; collapsed: boolean; navigate(command: string): void; about(): void }) {
  return <div className="sidebar-scroll settings-navigation">
    <button className="nav-button settings-back" aria-label="Back to app" title={collapsed ? "Back to app" : undefined} onClick={() => navigate("/chat")}><ArrowLeft size={17} /><span>Back to app</span></button>
    <div className="nav-group-label">Settings</div>
    <nav aria-label="Settings categories">{settingsRoutes.map(({ command, label, name, icon: Icon }) => <button className="nav-button" key={command} aria-label={name} title={collapsed ? label : undefined} aria-current={destination === command ? "page" : undefined} onClick={() => navigate(command)}><Icon size={17} /><span>{label}</span></button>)}</nav>
    <button className="nav-button" aria-label="About & updates" title={collapsed ? "About & updates" : undefined} onClick={about}><Info size={17} /><span>About &amp; updates</span></button>
  </div>;
}
