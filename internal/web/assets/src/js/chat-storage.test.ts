import { describe, expect, it } from "vitest";

import type { Turn } from "./chat-history";
import {
  clearStoredTurns,
  decodeStoredTurns,
  encodeStoredTurns,
  loadStoredTurns,
  MAX_STORAGE_BYTES,
  MAX_STORED_ANSWER,
  MAX_STORED_TURNS,
  saveStoredTurns,
  STORAGE_KEY,
  STORAGE_VERSION,
  type TurnStorage,
} from "./chat-storage";

interface MemoryStorage extends TurnStorage {
  values: Map<string, string>;
}

function memoryStorage(initial: Record<string, string> = {}): MemoryStorage {
  const values = new Map(Object.entries(initial));
  return {
    values,
    getItem(key) {
      return values.get(key) ?? null;
    },
    setItem(key, value) {
      values.set(key, value);
    },
    removeItem(key) {
      values.delete(key);
    },
  };
}

function turn(index: number, answer = `answer-${index}`): Turn {
  return {
    question: `question-${index}`,
    answer,
    status: index % 3 === 0 ? "stopped" : "complete",
  };
}

describe("chat storage", () => {
  it("round-trips complete and stopped turns", () => {
    const storage = memoryStorage();
    const turns = [turn(0), turn(1), turn(2)];

    saveStoredTurns(storage, turns);

    expect(storage.values.has(STORAGE_KEY)).toBe(true);
    expect(loadStoredTurns(storage)).toEqual(turns);
  });

  it("writes a versioned payload", () => {
    const payload = encodeStoredTurns([turn(1)]);
    expect(payload).not.toBeNull();

    const parsed = JSON.parse(payload ?? "{}");
    expect(parsed.version).toBe(STORAGE_VERSION);
    expect(parsed.turns).toHaveLength(1);
  });

  it("removes the key when there is nothing to store", () => {
    const storage = memoryStorage();
    saveStoredTurns(storage, [turn(1)]);
    saveStoredTurns(storage, []);

    expect(storage.values.has(STORAGE_KEY)).toBe(false);
    expect(encodeStoredTurns([])).toBeNull();
  });

  it("drops turns with empty content", () => {
    const storage = memoryStorage();
    saveStoredTurns(storage, [
      { question: " ", answer: "answer", status: "complete" },
      { question: "question", answer: "  ", status: "complete" },
      turn(1),
    ]);

    expect(loadStoredTurns(storage)).toEqual([turn(1)]);
  });

  it("decodes only the current version", () => {
    const payload = JSON.stringify({
      version: STORAGE_VERSION + 1,
      turns: [{ question: "q", answer: "a", status: "complete" }],
    });

    expect(decodeStoredTurns(payload)).toEqual([]);
  });

  it("tolerates corrupt and structurally invalid payloads", () => {
    expect(decodeStoredTurns("{")).toEqual([]);
    expect(decodeStoredTurns("null")).toEqual([]);
    expect(decodeStoredTurns("[]")).toEqual([]);
    expect(decodeStoredTurns(`{"version":${STORAGE_VERSION}}`)).toEqual([]);
    expect(decodeStoredTurns(`{"version":${STORAGE_VERSION},"turns":"nope"}`)).toEqual(
      [],
    );
  });

  it("skips invalid entries but keeps valid ones", () => {
    const payload = JSON.stringify({
      version: STORAGE_VERSION,
      turns: [
        { question: "q1", answer: "a1", status: "complete" },
        { question: 1, answer: "a2", status: "complete" },
        { question: "q3", answer: "a3", status: "unknown" },
        { question: "q4", answer: "a4", status: "stopped", extra: true },
      ],
    });

    expect(decodeStoredTurns(payload)).toEqual([
      { question: "q1", answer: "a1", status: "complete" },
      { question: "q4", answer: "a4", status: "stopped" },
    ]);
  });

  it("keeps only the newest turns", () => {
    const turns = Array.from({ length: MAX_STORED_TURNS + 10 }, (_, index) => turn(index));
    const storage = memoryStorage();

    saveStoredTurns(storage, turns);
    const restored = loadStoredTurns(storage);

    expect(restored).toHaveLength(MAX_STORED_TURNS);
    expect(restored[0]?.question).toBe(`question-${10}`);
    expect(restored.at(-1)?.question).toBe(`question-${MAX_STORED_TURNS + 9}`);
  });

  it("stays within the byte cap by dropping the oldest turns", () => {
    const answer = "x".repeat(8000);
    const turns = Array.from({ length: 10 }, (_, index) => turn(index, answer));
    const storage = memoryStorage();

    saveStoredTurns(storage, turns);
    const raw = storage.getItem(STORAGE_KEY);
    const restored = loadStoredTurns(storage);

    expect(raw).not.toBeNull();
    expect(new TextEncoder().encode(raw ?? "").length).toBeLessThanOrEqual(
      MAX_STORAGE_BYTES,
    );
    expect(restored.length).toBeGreaterThan(0);
    expect(restored.length).toBeLessThan(turns.length);
    expect(restored.at(-1)?.question).toBe(`question-${turns.length - 1}`);
  });

  it("truncates oversized answers and questions", () => {
    const payload = encodeStoredTurns([
      {
        question: "q".repeat(MAX_STORED_ANSWER),
        answer: "a".repeat(MAX_STORED_ANSWER + 5000),
        status: "complete",
      },
    ]);
    const [restored] = decodeStoredTurns(payload);

    expect(restored?.question).toHaveLength(500);
    expect(restored?.answer).toHaveLength(MAX_STORED_ANSWER);
  });

  it("clears the stored conversation", () => {
    const storage = memoryStorage();
    saveStoredTurns(storage, [turn(1)]);

    clearStoredTurns(storage);

    expect(storage.values.has(STORAGE_KEY)).toBe(false);
  });

  it("ignores unavailable storage", () => {
    const throwing: TurnStorage = {
      getItem() {
        throw new Error("blocked");
      },
      setItem() {
        throw new Error("blocked");
      },
      removeItem() {
        throw new Error("blocked");
      },
    };

    expect(loadStoredTurns(throwing)).toEqual([]);
    expect(() => saveStoredTurns(throwing, [turn(1)])).not.toThrow();
    expect(() => clearStoredTurns(throwing)).not.toThrow();
    expect(loadStoredTurns(null)).toEqual([]);
    expect(() => saveStoredTurns(null, [turn(1)])).not.toThrow();
  });
});
