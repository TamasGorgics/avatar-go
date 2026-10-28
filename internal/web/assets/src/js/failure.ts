export type FailureKind =
  | "rejected"
  | "rate_limited"
  | "busy"
  | "unavailable"
  | "network"
  | "timeout"
  | "stopped";

export interface FailureInfo {
  kind: FailureKind;
  message: string;
  canRetry: boolean;
  canEdit: boolean;
  retryAfterMs: number | null;
}

// A day is the longest window the server can describe (the daily cap). Cap
// hostile or broken values so a countdown can never run away.
export const MAX_RETRY_AFTER_MS = 24 * 60 * 60 * 1000;

// parseRetryAfter accepts both forms allowed by RFC 9110: delta-seconds and an
// HTTP-date. It returns milliseconds from now, or null when the header is
// missing or malformed.
export function parseRetryAfter(value: string | null, now: number): number | null {
  if (value === null) {
    return null;
  }
  const raw = value.trim();
  if (!raw) {
    return null;
  }

  if (/^\d+$/.test(raw)) {
    const seconds = Number(raw);
    if (!Number.isFinite(seconds)) {
      return null;
    }
    return Math.min(seconds * 1000, MAX_RETRY_AFTER_MS);
  }

  // Numeric-looking values that are not valid delta-seconds must not fall
  // through to Date.parse, which accepts a surprising range of strings.
  if (/^[+-]?\d/.test(raw)) {
    return null;
  }

  const at = Date.parse(raw);
  if (!Number.isFinite(at)) {
    return null;
  }
  return Math.min(Math.max(0, at - now), MAX_RETRY_AFTER_MS);
}

export function formatCountdown(remainingMs: number): string {
  const total = Math.max(0, Math.ceil(remainingMs / 1000));
  if (total < 60) {
    return `${total}s`;
  }

  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  if (minutes < 60) {
    return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
  }

  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest > 0 ? `${hours}h ${rest}m` : `${hours}h`;
}

export function rateLimitMessage(remainingMs: number): string {
  if (remainingMs <= 0) {
    return "Too many questions right now. You can retry now.";
  }
  return `Too many questions right now. Retry in ${formatCountdown(remainingMs)}.`;
}

export function failureForStatus(
  status: number,
  retryAfterMs: number | null,
  serverMessage = "",
): FailureInfo {
  const message = serverMessage.trim();

  if (status === 429) {
    if (retryAfterMs !== null) {
      return {
        kind: "rate_limited",
        message: rateLimitMessage(retryAfterMs),
        canRetry: true,
        canEdit: false,
        retryAfterMs,
      };
    }
    if (message && !message.startsWith("Another answer is still in progress")) {
      return {
        kind: "rate_limited",
        message,
        canRetry: true,
        canEdit: false,
        retryAfterMs: null,
      };
    }
    return {
      kind: "busy",
      message:
        message || "Another answer is still in progress. Please wait for it to finish.",
      canRetry: true,
      canEdit: false,
      retryAfterMs: null,
    };
  }

  if (status >= 400 && status < 500) {
    return {
      kind: "rejected",
      message: message || "That question can't be processed.",
      canRetry: false,
      canEdit: true,
      retryAfterMs: null,
    };
  }

  return {
    kind: "unavailable",
    message: message || "The twin is unavailable right now.",
    canRetry: true,
    canEdit: false,
    retryAfterMs: null,
  };
}

export function failureForNetwork(kind: "network" | "timeout"): FailureInfo {
  return {
    kind,
    message:
      kind === "timeout"
        ? "The twin took too long to answer. Please try again."
        : "Could not reach the twin. Check your connection and try again.",
    canRetry: true,
    canEdit: false,
    retryAfterMs: null,
  };
}

export function failureForEmptyAnswer(): FailureInfo {
  return {
    kind: "unavailable",
    message: "The twin did not return an answer. Please try again.",
    canRetry: true,
    canEdit: false,
    retryAfterMs: null,
  };
}

export function failureForStopped(): FailureInfo {
  return {
    kind: "stopped",
    message: "Answer stopped.",
    canRetry: true,
    canEdit: false,
    retryAfterMs: null,
  };
}
