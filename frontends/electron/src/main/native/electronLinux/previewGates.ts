export const LINUX_UNVALIDATED_NATIVE_SHELL_FLAG = "HCB_LINUX_ENABLE_UNVALIDATED_NATIVE_SHELL";

export function isLinuxUnvalidatedNativeShellEnabled(
  env: NodeJS.ProcessEnv = process.env
): boolean {
  const value = env[LINUX_UNVALIDATED_NATIVE_SHELL_FLAG]?.trim().toLowerCase();

  return value === "1" || value === "true" || value === "yes" || value === "on";
}

