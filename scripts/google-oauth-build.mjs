// Only Desktop application client material belongs in a distributed binary.
// These values are extractable and must never be used as server credentials.
export function googleOAuthFlags(env = process.env) {
  const id = env.GATOR_GOOGLE_DESKTOP_CLIENT_ID || "";
  const secret = env.GATOR_GOOGLE_DESKTOP_CLIENT_SECRET || "";
  if (!id && !secret) return [];
  if (id.length > 256 || !/^[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$/.test(id) ||
      secret.length > 256 || !/^[A-Za-z0-9_-]+$/.test(secret)) {
    throw new Error("Configure both GATOR_GOOGLE_DESKTOP_CLIENT_ID and GATOR_GOOGLE_DESKTOP_CLIENT_SECRET using a Google Desktop app client.");
  }
  return [`-X main.googleDesktopClientID=${id}`, `-X main.googleDesktopClientSecret=${secret}`];
}
