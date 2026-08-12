export const DANMAKU_SPEED_PX_PER_SEC = 145;
export const DANMAKU_LANE_GAP_PX = 40;
export const DANMAKU_STATIC_LEFT_PX = 16;
export const DANMAKU_STATIC_DURATION_MS = 2400;
export const DANMAKU_LANE_COUNT = 5;
export const DANMAKU_LANE_HEIGHT = 28;
export const DANMAKU_LANE_TOP = 8;
export const DANMAKU_FONT_SIZE_PX = 20;

export type LaneReservation = { lane: number; delayMs: number };

export function truncateDanmakuText(text: string, maxLength: number) {
  if (maxLength <= 0) return "";
  return Array.from(text).slice(0, maxLength).join("");
}

export function normalizeDanmakuText(text: string, maxLength: number) {
  return truncateDanmakuText(text.replace(/\s+/gu, " ").trim(), maxLength);
}

export function estimateDanmakuTextWidth(text: string) {
  let units = 0;
  for (const character of text) {
    units += /[\u0000-\u00ff]/.test(character) ? 0.55 : 1;
  }
  return Math.ceil(units * DANMAKU_FONT_SIZE_PX + 4);
}

export function reserveDanmakuLane({
  lanesBusyUntil,
  bulletWidth,
  reducedMotion,
  now,
  random = Math.random,
}: {
  lanesBusyUntil: number[];
  bulletWidth: number;
  reducedMotion: boolean;
  now: number;
  random?: () => number;
}): LaneReservation {
  const free = lanesBusyUntil
    .map((availableAt, lane) => ({ availableAt, lane }))
    .filter(({ availableAt }) => availableAt <= now);
  const choice =
    free.length > 0
      ? free[Math.min(Math.floor(random() * free.length), free.length - 1)]
      : lanesBusyUntil
          .map((availableAt, lane) => ({ availableAt, lane }))
          .sort((left, right) => left.availableAt - right.availableAt)[0];
  const lane = choice?.lane ?? 0;
  const startAt = Math.max(now, choice?.availableAt ?? 0);
  const occupiedFor = reducedMotion
    ? DANMAKU_STATIC_DURATION_MS
    : Math.round(
        ((bulletWidth + DANMAKU_LANE_GAP_PX) / DANMAKU_SPEED_PX_PER_SEC) * 1000,
      );
  lanesBusyUntil[lane] = startAt + occupiedFor;
  return { lane, delayMs: Math.max(0, startAt - now) };
}

export function playDanmakuBullet({
  element,
  stage,
  text,
  delayMs,
  reducedMotion,
  onFinish,
}: {
  element: HTMLSpanElement;
  stage: HTMLDivElement;
  text: string;
  delayMs: number;
  reducedMotion: boolean;
  onFinish: () => void;
}) {
  let animation: Animation | null = null;
  let finishTimer = 0;
  let startTimer = 0;
  let finished = false;

  const finish = () => {
    if (finished) return;
    finished = true;
    onFinish();
  };
  const start = () => {
    const bulletWidth = Math.max(
      element.offsetWidth,
      estimateDanmakuTextWidth(text),
    );
    if (reducedMotion) {
      element.style.left = `${DANMAKU_STATIC_LEFT_PX}px`;
      element.style.opacity = "0.8";
      finishTimer = window.setTimeout(finish, DANMAKU_STATIC_DURATION_MS);
      return;
    }

    const stageWidth = stage.clientWidth || window.innerWidth;
    const left = stageWidth + 8;
    const distance = left + bulletWidth + 8;
    element.style.left = `${left}px`;
    element.style.opacity = "0.8";
    animation = element.animate(
      [
        { transform: "translateX(0) translateZ(0)" },
        { transform: `translateX(${-distance}px) translateZ(0)` },
      ],
      {
        duration: Math.round((distance / DANMAKU_SPEED_PX_PER_SEC) * 1000),
        easing: "linear",
        fill: "forwards",
      },
    );
    animation.addEventListener("finish", finish);
  };

  if (delayMs > 0) startTimer = window.setTimeout(start, delayMs);
  else start();

  return () => {
    window.clearTimeout(startTimer);
    window.clearTimeout(finishTimer);
    animation?.removeEventListener("finish", finish);
    animation?.cancel();
  };
}

export function prefersReducedMotion() {
  return (
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false
  );
}
