import { useMemo } from "react";
import { syntaxTokens } from "./syntax";
export function SyntaxCode({ text, language = "text" }: { text: string; language?: string }) {
  const tokens = useMemo(() => syntaxTokens(text, language), [text, language]);
  return <code className={`syntax-code language-${language.replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 32)}`}>{tokens.map((token, i) => token.kind ? <span className={`syntax-${token.kind}`} key={i}>{token.text}</span> : token.text)}</code>;
}
