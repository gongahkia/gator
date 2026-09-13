import { Badge, Input } from "../../components/primitives";

export function parseTagText(value: string): string[] {
  const seen = new Set<string>();
  const tags: string[] = [];

  for (const raw of value.split(",")) {
    const tag = raw.trim();
    const key = tag.toLocaleLowerCase();

    if (!tag || seen.has(key)) {
      continue;
    }

    tags.push(tag);
    seen.add(key);
  }

  return tags;
}

export function TagInput({
  label = "Tags",
  onChange,
  value
}: {
  label?: string;
  onChange: (tags: string[]) => void;
  value: readonly string[] | undefined;
}): JSX.Element {
  return (
    <Input
      aria-label={label}
      label={label}
      onChange={(event) => onChange(parseTagText(event.target.value))}
      placeholder="focus, admin"
      value={(value ?? []).join(", ")}
    />
  );
}

export function TagBadges({ tags }: { tags: readonly string[] | undefined }): JSX.Element | null {
  if (!tags?.length) {
    return null;
  }

  return (
    <div className="flex flex-wrap gap-2">
      {tags.map((tag) => (
        <Badge key={tag} tone="info">
          {tag}
        </Badge>
      ))}
    </div>
  );
}
