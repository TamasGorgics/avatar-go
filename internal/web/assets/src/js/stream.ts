export interface StreamPacing {
  baseCps: number;
  liveMaxCps: number;
  drainSeconds: number;
  maxCps: number;
}

export const DEFAULT_PACING: StreamPacing = {
  baseCps: 70,
  liveMaxCps: 600,
  drainSeconds: 1.4,
  maxCps: 3000,
};

export interface TextStreamOptions {
  write: (text: string) => void;
  onPaint?: () => void;
  reducedMotion?: boolean;
  requestFrame?: (callback: FrameRequestCallback) => number;
  cancelFrame?: (handle: number) => void;
  now?: () => number;
  segment?: (text: string) => Iterable<string>;
  pacing?: Partial<StreamPacing>;
}

export interface TextStream {
  push(text: string): void;
  end(): void;
  cancel(): void;
  readonly drained: Promise<void>;
}

const graphemeSegmenter =
  typeof Intl !== "undefined" && typeof Intl.Segmenter === "function"
    ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
    : null;

export function splitGraphemes(text: string): string[] {
  if (graphemeSegmenter) {
    return Array.from(graphemeSegmenter.segment(text), (part) => part.segment);
  }
  return Array.from(text);
}

// Network chunks arrive unevenly. Queue graphemes and reveal them at a steady
// pace, speeding up as the backlog grows so the answer still catches up.
export function createTextStream(options: TextStreamOptions): TextStream {
  const {
    write,
    onPaint,
    reducedMotion = false,
    requestFrame = (callback) => requestAnimationFrame(callback),
    cancelFrame = (handle) => cancelAnimationFrame(handle),
    now = () => performance.now(),
    segment = splitGraphemes,
    pacing = {},
  } = options;

  const { baseCps, liveMaxCps, drainSeconds, maxCps } = { ...DEFAULT_PACING, ...pacing };

  const pending: string[] = [];
  let cursor = 0;
  let rendered = "";
  let frame: number | null = null;
  let lastFrame = 0;
  let carry = 0;
  let drainCps = baseCps;
  let ended = false;
  let resolveDrained!: () => void;

  const drained = new Promise<void>((resolve) => {
    resolveDrained = resolve;
  });

  const paint = (): void => {
    write(rendered);
    onPaint?.();
  };

  const resolveIfDrained = (): void => {
    if (ended && cursor >= pending.length) {
      resolveDrained();
    }
  };

  const step = (frameNow: number): void => {
    frame = null;
    const backlog = pending.length - cursor;
    if (backlog === 0) {
      resolveIfDrained();
      return;
    }

    const elapsed = Math.min(100, Math.max(0, frameNow - lastFrame));
    lastFrame = frameNow;

    const charsPerSecond = Math.min(
      ended
        ? drainCps
        : Math.max(baseCps, Math.min(liveMaxCps, backlog * 1.2)),
      maxCps,
    );

    carry += (charsPerSecond * elapsed) / 1000;
    const count = Math.min(backlog, Math.floor(carry));
    if (count > 0) {
      carry -= count;
      rendered += pending.slice(cursor, cursor + count).join("");
      cursor += count;
      paint();
    }

    if (cursor >= pending.length) {
      pending.length = 0;
      cursor = 0;
      carry = 0;
      resolveIfDrained();
      return;
    }

    frame = requestFrame(step);
  };

  const start = (): void => {
    if (frame !== null || cursor >= pending.length) {
      return;
    }
    lastFrame = now();
    frame = requestFrame(step);
  };

  const push = (text: string): void => {
    if (!text) {
      return;
    }
    if (reducedMotion) {
      rendered += text;
      paint();
      return;
    }
    for (const grapheme of segment(text)) {
      pending.push(grapheme);
    }
    start();
  };

  const end = (): void => {
    ended = true;
    const backlog = pending.length - cursor;
    drainCps = Math.max(baseCps, backlog / drainSeconds);
    if (reducedMotion || backlog === 0) {
      resolveDrained();
      return;
    }
    start();
  };

  const cancel = (): void => {
    ended = true;
    pending.length = 0;
    cursor = 0;
    if (frame !== null) {
      cancelFrame(frame);
      frame = null;
    }
    resolveDrained();
  };

  return { push, end, cancel, drained };
}
