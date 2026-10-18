const segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });

export function graphemeBoundaries(value: string): number[] {
  const positions = Array.from(segmenter.segment(value), (part) => part.index);
  return [...positions, value.length].filter((position, index, all) => index === 0 || position !== all[index - 1]);
}

export function previousGrapheme(value: string, cursor: number): number {
  return [...graphemeBoundaries(value)].reverse().find((position) => position < cursor) ?? 0;
}

export function nextGrapheme(value: string, cursor: number): number {
  return graphemeBoundaries(value).find((position) => position > cursor) ?? value.length;
}

export function graphemeAt(value: string, cursor: number): string {
  return value.slice(cursor, nextGrapheme(value, cursor));
}

export function graphemeColumn(value: string, cursor: number): number {
  return graphemeBoundaries(value).filter((position) => position < cursor).length;
}

export function cursorAtGraphemeColumn(value: string, column: number): number {
  const positions = graphemeBoundaries(value);
  return positions[Math.min(Math.max(0, column), positions.length - 1)] ?? 0;
}
