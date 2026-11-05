import { Braces } from "lucide-react";
import google from "../assets/connections/google.png";
import slack from "../assets/connections/slack.png";
import notion from "../assets/connections/notion.png";
import atlassian from "../assets/connections/atlassian.png";

const logos: Record<string, string> = { google_workspace: google, slack, notion, atlassian };

export function ConnectionLogo({ service }: { service: string }) {
  const source = logos[service];
  return <span className="connection-logo" aria-hidden="true">{source ? <img src={source} alt="" draggable={false} /> : <Braces size={22} />}</span>;
}
