import type { ChatMessage } from "./types";

export type TurnStatus = "complete" | "stopped";

// A turn is one completed question/answer pair. Failed attempts never become
// turns, so the transcript and the model history cannot disagree about what was
// successfully answered.
export interface Turn {
  question: string;
  answer: string;
  status: TurnStatus;
}

export const MAX_HISTORY = 6;

export function recentHistory(history: ChatMessage[]): ChatMessage[] {
  return history.slice(-MAX_HISTORY);
}

// turnsToHistory flattens completed turns into request history and appends the
// question currently being asked. Empty answers are skipped so a stopped or
// failed turn cannot leave two user messages in a row.
export function turnsToHistory(turns: readonly Turn[], pendingQuestion = ""): ChatMessage[] {
  const history: ChatMessage[] = [];

  for (const turn of turns) {
    if (!turn.question.trim()) {
      continue;
    }
    history.push({ role: "user", content: turn.question });
    if (turn.answer.trim()) {
      history.push({ role: "assistant", content: turn.answer });
    }
  }

  const question = pendingQuestion.trim();
  if (question) {
    history.push({ role: "user", content: question });
  }

  return recentHistory(history);
}
