export const MAX_QUESTION_LENGTH = 500;

export interface ChatElements {
  log: HTMLElement;
  empty: HTMLElement | null;
  clear: HTMLButtonElement | null;
  form: HTMLFormElement;
  input: HTMLInputElement;
  submit: HTMLButtonElement;
  submitLabel: HTMLElement | null;
  count: HTMLElement | null;
  status: HTMLElement | null;
  userTemplate: HTMLTemplateElement;
  assistantTemplate: HTMLTemplateElement;
}

export type MessageAction =
  | "stop"
  | "retry"
  | "edit"
  | "copy"
  | "regenerate"
  | "edit-last";

export type NoteTone = "muted" | "danger";

export function getChatElements(): ChatElements | null {
  const log = document.getElementById("chat-log");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  const submit = document.getElementById("chat-submit");
  const userTemplate = document.getElementById("user-message-template");
  const assistantTemplate = document.getElementById("assistant-message-template");
  const clear = document.getElementById("chat-clear");

  if (
    !(log instanceof HTMLElement) ||
    !(form instanceof HTMLFormElement) ||
    !(input instanceof HTMLInputElement) ||
    !(submit instanceof HTMLButtonElement) ||
    !(userTemplate instanceof HTMLTemplateElement) ||
    !(assistantTemplate instanceof HTMLTemplateElement)
  ) {
    return null;
  }

  return {
    log,
    empty: document.getElementById("chat-empty"),
    clear: clear instanceof HTMLButtonElement ? clear : null,
    form,
    input,
    submit,
    submitLabel: submit.querySelector<HTMLElement>('[data-role="submit-label"]'),
    count: document.getElementById("chat-count"),
    status: document.getElementById("chat-status"),
    userTemplate,
    assistantTemplate,
  };
}

export function updateCount(input: HTMLInputElement, count: HTMLElement | null): void {
  if (count) {
    count.textContent = `${input.value.length} / ${MAX_QUESTION_LENGTH}`;
  }
}

export function setStatus(status: HTMLElement | null, text: string): void {
  if (status) {
    status.textContent = text;
  }
}

export function scrollToEnd(
  log: HTMLElement,
  behavior: "auto" | "smooth",
  reducedMotion: boolean,
): void {
  if (behavior === "smooth" && !reducedMotion && typeof log.scrollTo === "function") {
    log.scrollTo({ top: log.scrollHeight, behavior: "smooth" });
    return;
  }
  log.scrollTop = log.scrollHeight;
}

export function setBusy(elements: ChatElements, busy: boolean): void {
  elements.input.disabled = busy;
  elements.submit.disabled = busy;
  elements.form.setAttribute("aria-busy", busy ? "true" : "false");
  if (elements.submitLabel) {
    elements.submitLabel.textContent = busy ? "Asking" : "Ask";
  }
}

export function appendMessage(
  elements: ChatElements,
  template: HTMLTemplateElement,
  text: string,
  reducedMotion: boolean,
): HTMLElement | null {
  const fragment = template.content.cloneNode(true) as DocumentFragment;
  const message = fragment.firstElementChild;
  if (!(message instanceof HTMLElement)) {
    return null;
  }

  const textNode = message.querySelector<HTMLElement>('[data-role="text"]');
  if (textNode) {
    textNode.textContent = text;
  }

  elements.log.append(fragment);
  if (elements.empty) {
    elements.empty.hidden = true;
  }
  scrollToEnd(elements.log, "smooth", reducedMotion);
  return message;
}

export function hasMessages(elements: ChatElements): boolean {
  return elements.log.querySelector('[data-role="message"]') !== null;
}

export function removeMessages(elements: ChatElements): void {
  for (const message of elements.log.querySelectorAll('[data-role="message"]')) {
    message.remove();
  }
}

export function setMessageText(message: HTMLElement | null, text: string): void {
  const textNode = message?.querySelector<HTMLElement>('[data-role="text"]');
  if (textNode) {
    textNode.textContent = text;
  }
}

export function setMessageNote(
  message: HTMLElement | null,
  text: string,
  tone: NoteTone = "muted",
): void {
  const note = message?.querySelector<HTMLElement>('[data-role="note"]');
  if (!note) {
    return;
  }
  if (!text) {
    note.hidden = true;
    note.textContent = "";
    return;
  }
  note.hidden = false;
  note.textContent = text;
  note.dataset.tone = tone;
}

export function actionButton(
  message: HTMLElement | null,
  action: MessageAction,
): HTMLButtonElement | null {
  return (
    message?.querySelector<HTMLButtonElement>(`button[data-action="${action}"]`) ?? null
  );
}

export function setActionText(
  message: HTMLElement | null,
  action: MessageAction,
  text: string,
): void {
  const button = actionButton(message, action);
  if (button) {
    button.textContent = text;
  }
}

export function showActions(
  message: HTMLElement | null,
  actions: readonly MessageAction[],
): void {
  if (!message) {
    return;
  }
  const row = message.querySelector<HTMLElement>('[data-role="actions"]');
  if (!row) {
    return;
  }

  const visible = new Set(actions);
  let any = false;
  for (const button of row.querySelectorAll<HTMLButtonElement>("button[data-action]")) {
    const action = button.dataset.action as MessageAction | undefined;
    const show = action !== undefined && visible.has(action);
    button.hidden = !show;
    if (show) {
      any = true;
    }
  }
  row.hidden = !any;
}

function removeTyping(message: HTMLElement): void {
  message.querySelector('[data-role="typing"]')?.remove();
}

export function markMessageStreaming(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "true";
  message.dataset.state = "streaming";
  delete message.dataset.error;
  setMessageNote(message, "");
}

export function markMessageComplete(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "false";
  message.dataset.state = "complete";
  delete message.dataset.error;
  removeTyping(message);
}

export function markMessageStopped(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "false";
  message.dataset.state = "stopped";
  delete message.dataset.error;
  removeTyping(message);
}

export function markMessageInterrupted(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "false";
  message.dataset.state = "interrupted";
  delete message.dataset.error;
  removeTyping(message);
}

export function markMessageError(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "false";
  message.dataset.state = "error";
  message.dataset.error = "true";
  removeTyping(message);
  setMessageNote(message, "");
}

// copyText uses the async clipboard API where available and falls back to a
// hidden textarea for insecure contexts and older browsers.
export async function copyText(text: string): Promise<boolean> {
  if (!text) {
    return false;
  }

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fall through to the textarea fallback.
  }

  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.append(area);
  const focused = document.activeElement;
  area.select();

  let copied = false;
  try {
    copied = document.execCommand("copy");
  } catch {
    copied = false;
  }
  area.remove();
  if (focused instanceof HTMLElement) {
    focused.focus();
  }
  return copied;
}
