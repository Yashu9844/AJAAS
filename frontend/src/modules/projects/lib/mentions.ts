/** Mentions travel as plain markdown text: @[Name](user:<uuid>). The server extracts and validates them. */

export type TextSegment = { kind: "text"; text: string } | { kind: "mention"; name: string; userId: string };

const MENTION_RE = /@\[([^\]\n]{1,80})\]\(user:([0-9a-fA-F-]{36})\)/g;

/** Splits a comment into plain text and mention tokens so both can be rendered as React text nodes. */
export function splitMentions(body: string): TextSegment[] {
  const out: TextSegment[] = [];
  let last = 0;
  for (const m of body.matchAll(MENTION_RE)) {
    const start = m.index ?? 0;
    if (start > last) out.push({ kind: "text", text: body.slice(last, start) });
    out.push({ kind: "mention", name: m[1], userId: m[2] });
    last = start + m[0].length;
  }
  if (last < body.length) out.push({ kind: "text", text: body.slice(last) });
  return out;
}

/** The token to insert for a person. Square brackets in names are removed so the token stays well formed. */
export function mentionToken(name: string, userId: string): string {
  const clean = name.replace(/[[\]\n]/g, "").trim().slice(0, 80) || "user";
  return `@[${clean}](user:${userId})`;
}
