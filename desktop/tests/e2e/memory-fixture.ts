// Ported from the retired terminal's integration fixture. The real foreground
// memory tool can issue a separate non-agent comparison request; it must not
// receive the outer conversation fixture's generic prose response.
export function memoryComparisonMessage(input: { messages: { role: string; content: unknown }[]; tools?: unknown[] }) {
  const last = input.messages.at(-1);
  if (input.tools?.length || last?.role !== "user" || typeof last.content !== "string") return;
  let data: { proposed_quote?: string; candidates?: { id: string; kind: string; subject: string }[] };
  try { data = JSON.parse(last.content); } catch { return; }
  if (!data.proposed_quote || !Array.isArray(data.candidates)) return;
  const withdrawn = data.candidates.find(candidate => candidate.kind === "withdrawn"), match = withdrawn ?? data.candidates[0];
  return { role: "assistant", content: JSON.stringify({ relation: withdrawn ? "withdrawn" : match ? "equivalent" : "distinct", match_ids: match ? [match.id] : [], suggested_subject: match?.subject || "preference.response_length", reason: "Synthetic memory protocol fixture." }) };
}
