import { turnsToHistory, type Turn } from "./chat-history";
import {
  clearStoredTurns,
  loadStoredTurns,
  saveStoredTurns,
  type TurnStorage,
} from "./chat-storage";
import {
  actionButton,
  appendMessage,
  copyText,
  hasMessages,
  markMessageComplete,
  markMessageError,
  markMessageInterrupted,
  markMessageStopped,
  markMessageStreaming,
  removeMessages,
  scrollToEnd,
  setActionText,
  setBusy,
  setMessageNote,
  setMessageText,
  setStatus,
  showActions,
  updateCount,
  type ChatElements,
  type MessageAction,
} from "./dom";
import {
  failureForEmptyAnswer,
  failureForNetwork,
  failureForStatus,
  failureForStopped,
  formatCountdown,
  parseRetryAfter,
  rateLimitMessage,
  type FailureInfo,
} from "./failure";
import { createTextStream, type TextStream } from "./stream";

export interface ChatController {
  ask(raw: string): Promise<void>;
  stop(): void;
  retry(): void;
  editFailed(): void;
  copy(button: HTMLButtonElement): Promise<void>;
  regenerate(): void;
  editLast(): void;
  clear(): void;
  readonly busy: boolean;
}

export const CLIENT_TIMEOUT_MS = 75_000;

const ANSWERING_STATUS = "Twin is answering.";

interface TurnRecord extends Turn {
  userEl: HTMLElement;
  assistantEl: HTMLElement;
}

interface Pending {
  question: string;
  userEl: HTMLElement;
  assistantEl: HTMLElement;
  base: Turn[];
  controller: AbortController;
  stream: TextStream | null;
  answer: string;
  stopped: boolean;
  timedOut: boolean;
  timeoutId: number | null;
}

interface FailureState {
  question: string;
  userEl: HTMLElement;
  assistantEl: HTMLElement;
  deadline: number | null;
  countdownId: number | null;
}

function sessionStorageOrNull(): TurnStorage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function toTurn(record: TurnRecord): Turn {
  return {
    question: record.question,
    answer: record.answer,
    status: record.status,
  };
}

export function createChat(elements: ChatElements, reducedMotion: boolean): ChatController {
  const storage = sessionStorageOrNull();

  let turns: TurnRecord[] = [];
  let pending: Pending | null = null;
  let failure: FailureState | null = null;
  let busy = false;

  const syncChrome = (): void => {
    const any = hasMessages(elements);
    if (elements.empty) {
      elements.empty.hidden = any;
    }
    if (elements.clear) {
      elements.clear.hidden = !any;
    }
  };

  const persist = (): void => {
    saveStoredTurns(
      storage,
      turns.map((turn) => toTurn(turn)),
    );
  };

  const stopCountdown = (state: FailureState): void => {
    if (state.countdownId !== null) {
      window.clearInterval(state.countdownId);
      state.countdownId = null;
    }
    state.deadline = null;
  };

  const dismissFailure = (): void => {
    const state = failure;
    if (!state) {
      return;
    }
    stopCountdown(state);
    showActions(state.assistantEl, []);
    failure = null;
  };

  const refreshTurnActions = (): void => {
    const canManageLast = !busy && failure === null;
    turns.forEach((turn, index) => {
      const actions: MessageAction[] = ["copy"];
      if (canManageLast && index === turns.length - 1) {
        actions.push("regenerate", "edit-last");
      }
      showActions(turn.assistantEl, actions);
    });
  };

  const appendUser = (question: string): HTMLElement | null =>
    appendMessage(elements, elements.userTemplate, question, reducedMotion);

  const appendAssistant = (): HTMLElement | null =>
    appendMessage(elements, elements.assistantTemplate, "", reducedMotion);

  const tickCountdown = (): void => {
    const state = failure;
    if (!state || state.deadline === null) {
      return;
    }

    const remaining = state.deadline - Date.now();
    const button = actionButton(state.assistantEl, "retry");
    if (remaining <= 0) {
      stopCountdown(state);
      if (button) {
        button.disabled = false;
        button.textContent = "Retry";
      }
      setMessageText(state.assistantEl, rateLimitMessage(0));
      setStatus(elements.status, "You can retry now.");
      return;
    }

    if (button) {
      button.textContent = `Retry in ${formatCountdown(remaining)}`;
    }
    setMessageText(state.assistantEl, rateLimitMessage(remaining));
  };

  const presentFailure = (
    attempt: Pick<Pending, "question" | "userEl" | "assistantEl">,
    info: FailureInfo,
  ): void => {
    dismissFailure();

    if (info.kind === "stopped") {
      markMessageStopped(attempt.assistantEl);
    } else {
      markMessageError(attempt.assistantEl);
    }
    setMessageText(attempt.assistantEl, info.message);

    const actions: MessageAction[] = [];
    if (info.canRetry) {
      actions.push("retry");
    }
    if (info.canEdit) {
      actions.push("edit");
    }
    showActions(attempt.assistantEl, actions);
    setActionText(attempt.assistantEl, "retry", "Retry");

    const state: FailureState = {
      question: attempt.question,
      userEl: attempt.userEl,
      assistantEl: attempt.assistantEl,
      deadline: null,
      countdownId: null,
    };
    failure = state;

    if (info.retryAfterMs !== null && info.retryAfterMs > 0) {
      state.deadline = Date.now() + info.retryAfterMs;
      const button = actionButton(state.assistantEl, "retry");
      if (button) {
        button.disabled = true;
      }
      tickCountdown();
      state.countdownId = window.setInterval(tickCountdown, 1000);
    }

    setStatus(elements.status, info.message);
    scrollToEnd(elements.log, "auto", reducedMotion);
  };

  const finishAttempt = (attempt: Pending): void => {
    if (pending !== attempt) {
      return;
    }
    if (attempt.timeoutId !== null) {
      window.clearTimeout(attempt.timeoutId);
      attempt.timeoutId = null;
    }
    attempt.stream = null;
    pending = null;
    busy = false;
    setBusy(elements, false);
    refreshTurnActions();
    syncChrome();
    elements.input.focus();
  };

  const commitComplete = (attempt: Pending): void => {
    if (!attempt.answer.trim()) {
      presentFailure(attempt, failureForEmptyAnswer());
      return;
    }

    setMessageText(attempt.assistantEl, attempt.answer);
    markMessageComplete(attempt.assistantEl);
    setMessageNote(attempt.assistantEl, "");
    turns.push({
      question: attempt.question,
      answer: attempt.answer,
      status: "complete",
      userEl: attempt.userEl,
      assistantEl: attempt.assistantEl,
    });
    persist();
    refreshTurnActions();
    setStatus(elements.status, "Answer complete.");
    scrollToEnd(elements.log, "auto", reducedMotion);
  };

  const commitStopped = (attempt: Pending): void => {
    if (!attempt.answer.trim()) {
      markMessageStopped(attempt.assistantEl);
      setMessageText(attempt.assistantEl, "Answer stopped.");
      presentFailure(attempt, failureForStopped());
      setStatus(elements.status, "Answer stopped.");
      return;
    }

    setMessageText(attempt.assistantEl, attempt.answer);
    markMessageStopped(attempt.assistantEl);
    setMessageNote(attempt.assistantEl, "Stopped by you.");
    turns.push({
      question: attempt.question,
      answer: attempt.answer,
      status: "stopped",
      userEl: attempt.userEl,
      assistantEl: attempt.assistantEl,
    });
    persist();
    refreshTurnActions();
    setStatus(elements.status, "Answer stopped. Partial answer kept.");
    scrollToEnd(elements.log, "auto", reducedMotion);
  };

  const settleInterrupted = (attempt: Pending, kind: "network" | "timeout"): void => {
    dismissFailure();
    attempt.stream?.cancel();
    setMessageText(attempt.assistantEl, attempt.answer);
    markMessageInterrupted(attempt.assistantEl);
    setMessageNote(
      attempt.assistantEl,
      kind === "timeout" ? "Answer timed out." : "Answer interrupted.",
      "danger",
    );
    showActions(attempt.assistantEl, ["retry", "copy"]);
    setActionText(attempt.assistantEl, "retry", "Retry");
    failure = {
      question: attempt.question,
      userEl: attempt.userEl,
      assistantEl: attempt.assistantEl,
      deadline: null,
      countdownId: null,
    };
    setStatus(
      elements.status,
      kind === "timeout"
        ? "Answer timed out. You can retry."
        : "Answer interrupted. You can retry.",
    );
    scrollToEnd(elements.log, "auto", reducedMotion);
  };

  const execute = async (attempt: Pending): Promise<void> => {
    try {
      const response = await fetch("/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ history: turnsToHistory(attempt.base, attempt.question) }),
        signal: attempt.controller.signal,
      });

      if (pending !== attempt) {
        return;
      }

      if (!response.ok) {
        const serverMessage = (await response.text()).trim();
        if (pending !== attempt) {
          return;
        }
        if (attempt.stopped) {
          commitStopped(attempt);
          return;
        }
        const retryAfterMs = parseRetryAfter(
          response.headers.get("Retry-After"),
          Date.now(),
        );
        presentFailure(
          attempt,
          failureForStatus(response.status, retryAfterMs, serverMessage),
        );
        return;
      }

      if (!response.body) {
        presentFailure(attempt, failureForNetwork("network"));
        return;
      }

      const textNode = attempt.assistantEl.querySelector<HTMLElement>('[data-role="text"]');
      const stream = createTextStream({
        write: (text) => {
          if (textNode) {
            textNode.textContent = text;
          }
        },
        onPaint: () => scrollToEnd(elements.log, "auto", reducedMotion),
        reducedMotion,
      });
      attempt.stream = stream;

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      for (;;) {
        const { done, value } = await reader.read();
        if (pending !== attempt) {
          return;
        }
        if (done) {
          break;
        }
        const text = decoder.decode(value, { stream: true });
        if (!text) {
          continue;
        }
        attempt.answer += text;
        stream.push(text);
      }

      const tail = decoder.decode();
      if (tail) {
        attempt.answer += tail;
        stream.push(tail);
      }
      stream.end();

      if (attempt.stopped) {
        commitStopped(attempt);
        return;
      }

      await stream.drained;
      if (pending !== attempt) {
        return;
      }
      if (attempt.stopped) {
        commitStopped(attempt);
        return;
      }
      commitComplete(attempt);
    } catch (error) {
      if (pending !== attempt) {
        return;
      }
      attempt.stream?.cancel();
      if (attempt.stopped) {
        commitStopped(attempt);
        return;
      }

      console.error("chat request failed", error);
      const kind = attempt.timedOut ? "timeout" : "network";
      if (attempt.answer.trim()) {
        settleInterrupted(attempt, kind);
      } else {
        presentFailure(attempt, failureForNetwork(kind));
      }
    } finally {
      finishAttempt(attempt);
    }
  };

  const startAttempt = (
    question: string,
    userEl: HTMLElement,
    assistantEl: HTMLElement,
    base: readonly Turn[],
  ): Promise<void> => {
    busy = true;
    setBusy(elements, true);
    markMessageStreaming(assistantEl);
    showActions(assistantEl, ["stop"]);
    setMessageText(assistantEl, "");
    setStatus(elements.status, ANSWERING_STATUS);
    refreshTurnActions();
    syncChrome();

    const controller = new AbortController();
    const attempt: Pending = {
      question,
      userEl,
      assistantEl,
      base: base.map((turn) => ({ ...turn })),
      controller,
      stream: null,
      answer: "",
      stopped: false,
      timedOut: false,
      timeoutId: null,
    };
    pending = attempt;
    attempt.timeoutId = window.setTimeout(() => {
      if (pending !== attempt || attempt.stopped) {
        return;
      }
      attempt.timedOut = true;
      controller.abort();
    }, CLIENT_TIMEOUT_MS);
    return execute(attempt);
  };

  const ask = async (raw: string): Promise<void> => {
    if (busy) {
      return;
    }
    const question = raw.trim();
    if (!question) {
      return;
    }

    dismissFailure();
    elements.input.value = "";
    updateCount(elements.input, elements.count);

    const userEl = appendUser(question);
    const assistantEl = appendAssistant();
    if (!userEl || !assistantEl) {
      return;
    }

    await startAttempt(question, userEl, assistantEl, turns.map((turn) => toTurn(turn)));
  };

  const stop = (): void => {
    const attempt = pending;
    if (!attempt || attempt.stopped) {
      return;
    }
    attempt.stopped = true;
    showActions(attempt.assistantEl, []);
    setStatus(elements.status, "Stopping the answer.");
    attempt.controller.abort();
    attempt.stream?.cancel();
  };

  const retry = (): void => {
    if (busy || !failure) {
      return;
    }
    if (failure.deadline !== null && Date.now() < failure.deadline) {
      return;
    }

    const state = failure;
    dismissFailure();
    state.assistantEl.remove();
    syncChrome();

    const assistantEl = appendAssistant();
    if (!assistantEl) {
      return;
    }
    void startAttempt(
      state.question,
      state.userEl,
      assistantEl,
      turns.map((turn) => toTurn(turn)),
    );
  };

  const editFailed = (): void => {
    if (busy || !failure) {
      return;
    }

    const state = failure;
    dismissFailure();
    state.assistantEl.remove();
    state.userEl.remove();
    elements.input.value = state.question;
    updateCount(elements.input, elements.count);
    syncChrome();
    refreshTurnActions();
    setStatus(elements.status, "Question ready to edit.");
    elements.input.focus();
    elements.input.select();
  };

  const copy = async (button: HTMLButtonElement): Promise<void> => {
    const message = button.closest<HTMLElement>('[data-role="message"]');
    const answer = message?.querySelector<HTMLElement>('[data-role="text"]')?.textContent ?? "";
    if (!answer.trim()) {
      setStatus(elements.status, "There is no answer to copy.");
      return;
    }

    const copied = await copyText(answer);
    if (!copied) {
      setStatus(elements.status, "Could not copy the answer. Select the text manually.");
      return;
    }

    setStatus(elements.status, "Answer copied.");
    button.textContent = "Copied";
    window.setTimeout(() => {
      if (button.isConnected) {
        button.textContent = "Copy";
      }
    }, 1500);
  };

  const regenerate = (): void => {
    if (busy) {
      return;
    }
    dismissFailure();

    const last = turns[turns.length - 1];
    if (!last) {
      return;
    }
    turns.pop();
    last.assistantEl.remove();
    persist();
    refreshTurnActions();
    syncChrome();

    const assistantEl = appendAssistant();
    if (!assistantEl) {
      return;
    }
    void startAttempt(
      last.question,
      last.userEl,
      assistantEl,
      turns.map((turn) => toTurn(turn)),
    );
  };

  const editLast = (): void => {
    if (busy) {
      return;
    }
    dismissFailure();

    const last = turns.pop();
    if (!last) {
      return;
    }
    last.userEl.remove();
    last.assistantEl.remove();
    persist();
    refreshTurnActions();
    syncChrome();
    elements.input.value = last.question;
    updateCount(elements.input, elements.count);
    setStatus(elements.status, "Last question ready to edit.");
    elements.input.focus();
    elements.input.select();
  };

  const clear = (): void => {
    const attempt = pending;
    if (attempt) {
      pending = null;
      if (attempt.timeoutId !== null) {
        window.clearTimeout(attempt.timeoutId);
        attempt.timeoutId = null;
      }
      attempt.controller.abort();
      attempt.stream?.cancel();
    }

    dismissFailure();
    turns = [];
    removeMessages(elements);
    clearStoredTurns(storage);
    busy = false;
    setBusy(elements, false);
    elements.input.value = "";
    updateCount(elements.input, elements.count);
    syncChrome();
    setStatus(elements.status, "Conversation cleared.");
    elements.input.focus();
  };

  const restore = (): void => {
    for (const turn of loadStoredTurns(storage)) {
      const userEl = appendMessage(elements, elements.userTemplate, turn.question, true);
      const assistantEl = appendMessage(
        elements,
        elements.assistantTemplate,
        turn.answer,
        true,
      );
      if (!userEl || !assistantEl) {
        continue;
      }

      if (turn.status === "stopped") {
        markMessageStopped(assistantEl);
        setMessageNote(assistantEl, "Stopped by you.");
      } else {
        markMessageComplete(assistantEl);
      }
      turns.push({ ...turn, userEl, assistantEl });
    }

    syncChrome();
    refreshTurnActions();
    if (turns.length > 0) {
      setStatus(elements.status, "Conversation restored.");
    }
  };

  restore();

  return {
    ask,
    stop,
    retry,
    editFailed,
    copy,
    regenerate,
    editLast,
    clear,
    get busy() {
      return busy;
    },
  };
}
