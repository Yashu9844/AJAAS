"use client";

import { usePeople } from "../../hooks/use-people";
import { splitMentions } from "../../lib/mentions";

/**
 * Shows markdown source as plain text (line breaks kept) and turns @[Name](user:id) mention tokens into a highlighted
 * name. Everything is rendered as React text nodes; no HTML is ever injected.
 */
export function MarkdownText({ text }: { text: string }) {
  const { nameOf } = usePeople();
  return (
    <p className="pm-text">
      {splitMentions(text).map((seg, i) =>
        seg.kind === "text" ? (
          <span key={i}>{seg.text}</span>
        ) : (
          <span key={i} className="pm-mention" title={nameOf(seg.userId)}>
            @{seg.name}
          </span>
        ),
      )}
    </p>
  );
}
