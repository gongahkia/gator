export type FrontendPreferenceValues = { sidebarCollapsed: boolean; groupChatsByFolder: boolean; workDetailsSections: { outputs: boolean; context: boolean; connections: boolean; workspace: boolean } }
export function frontendDefaults(): FrontendPreferenceValues
export class FrontendPreferences {
  constructor(userData: string)
  read(): Promise<FrontendPreferenceValues>
  configure(patch: Partial<FrontendPreferenceValues>): Promise<FrontendPreferenceValues>
  flush(): Promise<void>
}
