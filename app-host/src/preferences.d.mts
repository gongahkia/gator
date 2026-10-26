export type FrontendPreferences = { sidebarCollapsed: boolean; groupChatsByFolder: boolean; workDetailsSections: { outputs: boolean; context: boolean; connections: boolean; workspace: boolean } }
export function frontendDefaults(): FrontendPreferences
export class FrontendPreferences {
  constructor(userData: string)
  read(): Promise<FrontendPreferences>
  configure(patch: Partial<FrontendPreferences>): Promise<FrontendPreferences>
  flush(): Promise<void>
}
