// The real foreground tool now makes a separate, non-agent comparison request.
// Keep this local protocol fixture distinct from the outer tool-calling agent.
export function memoryComparisonMessage(input: { messages: { role: string; content: string }[]; tools?: unknown[] }) {
  if (input.tools?.length || input.messages.at(-1)?.role !== "user") return undefined
  let data: { proposed_quote?: string; candidates?: { id: string; kind: string; subject: string }[] }
  try { data = JSON.parse(input.messages.at(-1)!.content) } catch { return undefined }
  if (!data.proposed_quote || !Array.isArray(data.candidates)) return undefined
  const withdrawn = data.candidates.find(candidate => candidate.kind === "withdrawn")
  const match = withdrawn ?? data.candidates[0]
  return { role: "assistant", content: JSON.stringify({ relation: withdrawn ? "withdrawn" : match ? "equivalent" : "distinct", match_ids: match ? [match.id] : [], suggested_subject: match?.subject || "preference.response_length", reason: "Synthetic memory protocol fixture." }) }
}
