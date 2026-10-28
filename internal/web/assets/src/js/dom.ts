export const MAX_QUESTION_LENGTH = 500;

export interface ChatElements {
  log: HTMLElement;
  empty: HTMLElement | null;
  form: HTMLFormElement;
  input: HTMLInputElement;
  submit: HTMLButtonElement;
  submitLabel: HTMLElement | null;
  count: HTMLElement | null;
  status: HTMLElement | null;
  userTemplate: HTMLTemplateElement;
  assistantTemplate: HTMLTemplateElement;
}

export function getChatElements(): ChatElements | null {
  const log = document.getElementById("chat-log");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  const submit = document.getElementById("chat-submit");
  const userTemplate = document.getElementById("user-message-template");
  const assistantTemplate = document.getElementById("assistant-message-template");

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

export function finishMessage(message: HTMLElement | null): void {
  if (!message) {
    return;
  }
  message.dataset.streaming = "false";
  message.querySelector('[data-role="typing"]')?.remove();
}

export function failMessage(message: HTMLElement | null, text: string): void {
  if (!message) {
    return;
  }
  message.dataset.error = "true";
  message.dataset.streaming = "false";
  message.querySelector('[data-role="typing"]')?.remove();
  const textNode = message.querySelector<HTMLElement>('[data-role="text"]');
  if (textNode) {
    textNode.textContent = text;
  }
}
