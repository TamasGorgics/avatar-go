import type { Turn, TurnStatus } from "./chat-history";

export const STORAGE_KEY = "avatar-go:chat:v1";
export const STORAGE_VERSION = 1;
export const MAX_STORED_TURNS = 50;
export const MAX_STORAGE_BYTES = 32 * 1024;
export const MAX_STORED_QUESTION = 500;
export const MAX_STORED_ANSWER = 8 * 1024;

export interface TurnStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface StoredTurn {
  question: string;
  answer: string;
  status: TurnStatus;
}

interface StoredConversation {
  version: number;
  turns: StoredTurn[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function byteLength(text: string): number {
  return new TextEncoder().encode(text).length;
}

function isStatus(value: unknown): value is TurnStatus {
  return value === "complete" || value === "stopped";
}

function normalizeTurn(turn: Turn): StoredTurn | null {
  const question = turn.question.trim();
  if (!question || !turn.answer.trim() || !isStatus(turn.status)) {
    return null;
  }
  return {
    question: question.slice(0, MAX_STORED_QUESTION),
    answer: turn.answer.slice(0, MAX_STORED_ANSWER),
    status: turn.status,
  };
}

// encodeStoredTurns keeps the newest turns that fit in both the turn and byte
// caps. It returns null when there is nothing worth storing, which tells the
// caller to remove the key instead of writing an empty shell.
export function encodeStoredTurns(turns: readonly Turn[]): string | null {
  let kept = turns
    .map(normalizeTurn)
    .filter((turn): turn is StoredTurn => turn !== null)
    .slice(-MAX_STORED_TURNS);

  while (kept.length > 0) {
    const conversation: StoredConversation = { version: STORAGE_VERSION, turns: kept };
    const payload = JSON.stringify(conversation);
    if (byteLength(payload) <= MAX_STORAGE_BYTES) {
      return payload;
    }
    kept = kept.slice(1);
  }

  return null;
}

export function decodeStoredTurns(raw: string | null): Turn[] {
  if (!raw) {
    return [];
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }

  if (
    !isRecord(parsed) ||
    parsed.version !== STORAGE_VERSION ||
    !Array.isArray(parsed.turns)
  ) {
    return [];
  }

  const turns: Turn[] = [];
  for (const value of parsed.turns) {
    if (!isRecord(value)) {
      continue;
    }
    const { question, answer, status } = value;
    if (typeof question !== "string" || typeof answer !== "string" || !isStatus(status)) {
      continue;
    }
    if (!question.trim() || !answer.trim()) {
      continue;
    }
    turns.push({
      question: question.slice(0, MAX_STORED_QUESTION),
      answer: answer.slice(0, MAX_STORED_ANSWER),
      status,
    });
  }

  return turns.slice(-MAX_STORED_TURNS);
}

export function loadStoredTurns(storage: TurnStorage | null): Turn[] {
  if (!storage) {
    return [];
  }
  try {
    return decodeStoredTurns(storage.getItem(STORAGE_KEY));
  } catch {
    return [];
  }
}

export function saveStoredTurns(storage: TurnStorage | null, turns: readonly Turn[]): void {
  if (!storage) {
    return;
  }
  try {
    const payload = encodeStoredTurns(turns);
    if (payload === null) {
      storage.removeItem(STORAGE_KEY);
      return;
    }
    storage.setItem(STORAGE_KEY, payload);
  } catch {
    // Storage can be unavailable or full. The in-page transcript still works.
  }
}

export function clearStoredTurns(storage: TurnStorage | null): void {
  if (!storage) {
    return;
  }
  try {
    storage.removeItem(STORAGE_KEY);
  } catch {
    // Ignore unavailable storage.
  }
}
