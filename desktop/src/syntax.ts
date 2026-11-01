// A bounded lexical highlighter, never an HTML interpreter or language executor.
// Unknown languages remain plain text; this deliberately isn't an IDE parser.
export type SyntaxToken = { text: string; kind?: "keyword" | "string" | "comment" | "number" | "literal" };
const keywords = new Set("as async await break case catch class const continue def default defer del do else elif enum except export extends finally fn for from func function go if impl import in interface let match module namespace new package pass private public raise range return select static struct super switch throw trait try type typeof use var void while with yield".split(" "));
const languages = new Set("js javascript jsx ts typescript tsx go python py rust rs java c cpp csharp cs swift kotlin kt ruby rb bash sh shell zsh json jsonc yaml yml sql css scss html xml".split(" "));
const hashComments = new Set("python py ruby rb bash sh shell zsh yaml yml".split(" "));
export function syntaxTokens(text: string, language = "text"): SyntaxToken[] {
  if (!languages.has(language.toLowerCase())) return [{ text }];
  const tokens: SyntaxToken[] = [], limit = Math.min(text.length, 65536);
  let i = 0, plain = 0;
  const push = (end: number, kind: SyntaxToken["kind"]) => { if (i > plain) tokens.push({ text: text.slice(plain, i) }); tokens.push({ text: text.slice(i, end), kind }); i = end; plain = i; };
  const word = /[A-Za-z_$][\w$]*/y, number = /\b(?:0x[\da-f]+|\d+(?:\.\d+)?(?:e[+-]?\d+)?)\b/iy;
  while (i < limit && tokens.length < 16000) {
    const character = text[i], next = text[i + 1];
    if (character === "'" || character === '"' || character === "`") {
      let end = i + 1; while (end < limit) { if (text[end] === "\\") { end += 2; continue; } if (text[end++] === character) break; } push(Math.min(end, limit), "string"); continue;
    }
    if (character === "/" && next === "/" || character === "#" && hashComments.has(language.toLowerCase())) { let end = text.indexOf("\n", i); if (end < 0 || end > limit) end = limit; push(end, "comment"); continue; }
    if (character === "/" && next === "*") { const end = text.indexOf("*/", i + 2); push(end < 0 ? limit : Math.min(limit, end + 2), "comment"); continue; }
    word.lastIndex = i; const found = word.exec(text);
    if (found) { const value = found[0], kind = keywords.has(value) ? "keyword" : /^(true|false|null|nil|None|True|False|undefined)$/.test(value) ? "literal" : undefined; if (kind) push(i + value.length, kind); else i += value.length; continue; }
    number.lastIndex = i; const numeric = number.exec(text); if (numeric) { push(i + numeric[0].length, "number"); continue; }
    i++;
  }
  if (plain < text.length) tokens.push({ text: text.slice(plain) });
  return tokens;
}
