export type FrontendPreferenceValues = import("../contracts/protocol").TerminalPreferences
export function validFrontendPreferences(value: unknown): boolean
export function frontendDefaults(): FrontendPreferenceValues
export class FrontendPreferences {
  constructor(userData: string)
  read(): Promise<FrontendPreferenceValues>
  configure(patch: Partial<FrontendPreferenceValues>): Promise<FrontendPreferenceValues>
  flush(): Promise<void>
}
