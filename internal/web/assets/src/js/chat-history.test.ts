import { describe, expect, it } from "vitest";

import { MAX_HISTORY, recentHistory, turnsToHistory, type Turn } from "./chat-history";
import type { ChatMessage } from "./types";

function historyOf(length: number): ChatMessage[] {
  return Array.from({ length }, (_, index) => ({
    role: index % 2 === 0 ? "user" : "assistant",
    content: `message-${index}`,
  }));
}

function turn(index: number, status: Turn["status"] = "complete"): Turn {
  return {
    question: `question-${index}`,
    answer: `answer-${index}`,
    status,
  };
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

describe("turnsToHistory", () => {
  it("flattens completed turns into alternating messages", () => {
    expect(turnsToHistory([turn(1), turn(2)])).toEqual([
      { role: "user", content: "question-1" },
      { role: "assistant", content: "answer-1" },
      { role: "user", content: "question-2" },
      { role: "assistant", content: "answer-2" },
    ]);
  });

  it("appends the pending question", () => {
    expect(turnsToHistory([turn(1)], "  follow-up  ")).toEqual([
      { role: "user", content: "question-1" },
      { role: "assistant", content: "answer-1" },
      { role: "user", content: "follow-up" },
    ]);
  });

  it("keeps stopped partial answers", () => {
    expect(turnsToHistory([turn(1, "stopped")], "next")).toEqual([
      { role: "user", content: "question-1" },
      { role: "assistant", content: "answer-1" },
      { role: "user", content: "next" },
    ]);
  });

  it("skips empty answers and questions", () => {
    const turns: Turn[] = [
      { question: "q1", answer: "  ", status: "complete" },
      { question: "   ", answer: "a2", status: "complete" },
      { question: "q3", answer: "a3", status: "complete" },
    ];

    expect(turnsToHistory(turns)).toEqual([
      { role: "user", content: "q1" },
      { role: "user", content: "q3" },
      { role: "assistant", content: "a3" },
    ]);
  });

  it("keeps only the last six messages with the pending question last", () => {
    const turns = Array.from({ length: 10 }, (_, index) => turn(index));
    const got = turnsToHistory(turns, "pending");

    expect(got).toHaveLength(MAX_HISTORY);
    expect(got.at(-1)).toEqual({ role: "user", content: "pending" });
    expect(got[0]).toEqual({ role: "assistant", content: "answer-7" });
  });

  it("returns a fresh array and does not mutate the turns", () => {
    const turns = [turn(1)];
    const got = turnsToHistory(turns);
    got.push({ role: "user", content: "extra" });

    expect(turns).toHaveLength(1);
    expect(turnsToHistory(turns)).toHaveLength(2);
  });
});
