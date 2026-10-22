import type { Page } from "@playwright/test";

// Exercise the visible secondary navigation, including its drawer/focus behavior.
export async function openSurface(page: Page, name: "Saved knowledge" | "Follow-up checks" | "Overview" | "Conversations" | "Ongoing goals" | "Inbox" | "Task runs") {
  const saved = ["Saved knowledge", "Conversations", "Task runs"].includes(name);
  const trigger = page.getByRole("button", { name: saved ? "Saved" : "Following", exact: true });
  if (await trigger.getAttribute("aria-expanded") !== "true") await trigger.click();
  const label = { "Saved knowledge": "What Gator remembers", "Follow-up checks": "Manage checks", "Overview": "Activity & attention", "Conversations": "All conversations", "Ongoing goals": "Open goals", "Inbox": "Updates", "Task runs": "Task runs" }[name];
  await page.locator(".library-drawer").getByRole("button", { name: new RegExp(`^${label}`) }).click();
}


// Command search is intentionally opened through the configured shortcut.
export async function openCommandSearch(page: Page) {
  const binding=await page.evaluate(async()=> (await window.gator!.snapshot()).state?.desktop?.shortcuts?.command_search ?? "Mod+K");
  const shortcut=binding.split("+").map(part=>part==="Mod" ? process.platform==="darwin" ? "Meta" : "Control" : part==="Ctrl" ? "Control" : part).join("+");
  await page.keyboard.press(shortcut);
}
