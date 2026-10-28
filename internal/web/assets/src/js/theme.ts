export type Theme = "light" | "dark";

const STORAGE_KEY = "theme";
const DARK_QUERY = "(prefers-color-scheme: dark)";

const themeColors: Record<Theme, string> = {
  light: "#fbfaf7",
  dark: "#0e171c",
};

export function isTheme(value: string | null | undefined): value is Theme {
  return value === "light" || value === "dark";
}

export function resolveTheme(stored: string | null, systemDark: boolean): Theme {
  if (isTheme(stored)) {
    return stored;
  }
  return systemDark ? "dark" : "light";
}

function readStoredTheme(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function storeTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Storage can be unavailable in private modes; keep the in-page choice.
  }
}

function currentTheme(): Theme {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.dataset.theme = theme;

  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute("content", themeColors[theme]);
  }

  const toggle = document.getElementById("theme-toggle");
  if (toggle) {
    toggle.setAttribute("aria-checked", theme === "dark" ? "true" : "false");
    toggle.setAttribute("title", theme === "dark" ? "Switch to light theme" : "Switch to dark theme");
  }
}

export function setupTheme(): void {
  const media = window.matchMedia?.(DARK_QUERY) ?? null;
  const stored = readStoredTheme();
  let explicit = isTheme(stored);
  const applied = document.documentElement.dataset.theme;
  const initial = isTheme(applied) ? applied : resolveTheme(stored, media?.matches ?? false);

  applyTheme(initial);

  document.getElementById("theme-toggle")?.addEventListener("click", () => {
    const next = currentTheme() === "dark" ? "light" : "dark";
    explicit = true;
    storeTheme(next);
    applyTheme(next);
  });

  media?.addEventListener("change", (event) => {
    if (!explicit) {
      applyTheme(event.matches ? "dark" : "light");
    }
  });

  window.addEventListener("storage", (event) => {
    if (event.key !== STORAGE_KEY) {
      return;
    }
    explicit = isTheme(event.newValue);
    applyTheme(resolveTheme(event.newValue, media?.matches ?? false));
  });
}
