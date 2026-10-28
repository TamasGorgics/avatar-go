import { describe, expect, it } from "vitest";

import { MAX_HISTORY, recentHistory } from "./chat";
import type { ChatMessage } from "./types";

function historyOf(length: number): ChatMessage[] {
  return Array.from({ length }, (_, index) => ({
    role: index % 2 === 0 ? "user" : "assistant",
    content: `message-${index}`,
  }));
}

describe("recentHistory", () => {
  it("keeps only the last six messages", () => {
    const history = historyOf(10);
    const got = recentHistory(history);

    expect(got).toHaveLength(MAX_HISTORY);
    expect(got.map((message) => message.content)).toEqual([
      "message-4",
      "message-5",
      "message-6",
      "message-7",
      "message-8",
      "message-9",
    ]);
  });

  it("leaves shorter histories unchanged", () => {
    const history = historyOf(3);
    expect(recentHistory(history)).toEqual(history);
  });

  it("does not mutate the original history", () => {
    const history = historyOf(10);
    recentHistory(history);
    expect(history).toHaveLength(10);
  });
});
