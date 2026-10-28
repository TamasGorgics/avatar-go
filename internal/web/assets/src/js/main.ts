import { createChat } from "./chat";
import { getChatElements, updateCount, type MessageAction } from "./dom";
import { setupTheme } from "./theme";

const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

setupTheme();

const elements = getChatElements();
if (elements) {
  const chat = createChat(elements, reduceMotion);

  elements.form.addEventListener("submit", (event) => {
    event.preventDefault();
    void chat.ask(elements.input.value);
  });

  elements.clear?.addEventListener("click", () => {
    chat.clear();
  });

  elements.log.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    const button = target.closest<HTMLButtonElement>("button[data-action]");
    if (!button) {
      return;
    }

    const action = button.dataset.action as MessageAction | undefined;
    switch (action) {
      case "stop":
        chat.stop();
        break;
      case "retry":
        chat.retry();
        break;
      case "edit":
        chat.editFailed();
        break;
      case "copy":
        void chat.copy(button);
        break;
      case "regenerate":
        chat.regenerate();
        break;
      case "edit-last":
        chat.editLast();
        break;
    }
  });

  elements.input.addEventListener("input", () => {
    updateCount(elements.input, elements.count);
  });

  for (const button of document.querySelectorAll<HTMLElement>("[data-prompt]")) {
    button.addEventListener("click", () => {
      if (chat.busy) {
        return;
      }
      elements.input.value = button.dataset.prompt ?? "";
      updateCount(elements.input, elements.count);
      elements.input.focus();
    });
  }

  updateCount(elements.input, elements.count);
}
