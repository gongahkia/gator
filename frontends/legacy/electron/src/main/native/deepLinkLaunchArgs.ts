import { parseHotCrossBunsDeepLink } from "./deepLinks";
import { HCB_DEEP_LINK_SCHEME } from "./types";

const deepLinkPrefix = `${HCB_DEEP_LINK_SCHEME}://`;

export function extractHotCrossBunsDeepLinksFromArgv(argv: readonly string[]): string[] {
  const links: string[] = [];
  const seen = new Set<string>();

  for (const arg of argv) {
    const candidate = arg.trim();

    if (!hasHotCrossBunsScheme(candidate) || !parseHotCrossBunsDeepLink(candidate) || seen.has(candidate)) {
      continue;
    }

    seen.add(candidate);
    links.push(candidate);
  }

  return links;
}

function hasHotCrossBunsScheme(value: string): boolean {
  return value.slice(0, deepLinkPrefix.length).toLowerCase() === deepLinkPrefix;
}
