"use client";

import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from "react";
import {
  LuChevronDown,
  LuMessageCircleMore,
  LuSearch,
  LuSmilePlus,
} from "react-icons/lu";

import {
  DANMAKU_FONT_SIZE_PX,
  DANMAKU_LANE_COUNT,
  DANMAKU_LANE_HEIGHT,
  DANMAKU_LANE_TOP,
  estimateDanmakuTextWidth,
  normalizeDanmakuText,
  playDanmakuBullet,
  prefersReducedMotion,
  reserveDanmakuLane,
  truncateDanmakuText,
} from "./danmaku-motion";
import { useMoreReactionsPresence } from "./use-presence-motion";

export type DanmakuMessage = {
  id: string;
  text: string;
  self?: boolean;
};

export type DanmakuLabels = {
  closeInteractiveLayer: string;
  collapseBar: string;
  enterToSend: string;
  expandBar: string;
  frequentlyUsed: string;
  moreReactions: string;
  openComposer: string;
  sayHi: string;
  searchEmoji: string;
  sendReaction: (emoji: string) => string;
  sent: string;
  submitFailed: string;
};

export type DanmakuProps = {
  className?: string;
  labels: DanmakuLabels;
  maxLength?: number;
  messages: DanmakuMessage[];
  onRequestComposer: () => Promise<boolean>;
  onSend: (text: string) => Promise<DanmakuMessage | null>;
};

type ActiveMessage = DanmakuMessage & {
  delayMs: number;
  instanceId: string;
  reducedMotion: boolean;
  top: number;
};

const MAX_VISIBLE = 40;
const SEED_INTERVAL_MS = 2400;
const AUTO_COLLAPSE_MS = 4000;
const GREETING_AUTO_DISMISS_MS = 3500;
const LONG_PRESS_DELAY_MS = 420;
const LONG_PRESS_REPEAT_MS = 220;
const QUICK_REACTIONS = ["❤️", "👍", "🔥", "👏", "🙌", "👀"] as const;
const MORE_REACTIONS = [
  "💯",
  "🎉",
  "✅",
  "❌",
  "👀",
  "✨",
  "🚀",
  "➕",
  "🙏",
  "🔥",
  "😆",
  "🤔",
  "😱",
  "👋",
  "🌈",
  "❤️",
  "👏",
  "🐞",
] as const;
const SEARCH_TERMS: Record<(typeof MORE_REACTIONS)[number], string> = {
  "💯": "100 perfect 满分 百分百",
  "🎉": "party celebrate 庆祝 派对",
  "✅": "check yes correct 对 正确",
  "❌": "x no wrong 错误 不",
  "👀": "eyes look 眼睛 看",
  "✨": "sparkles shine 闪光 闪耀",
  "🚀": "rocket launch 火箭 发射",
  "➕": "plus add 加 添加",
  "🙏": "pray thanks please 谢谢 拜托",
  "🔥": "fire hot 火 热",
  "😆": "laugh smile 笑 开心",
  "🤔": "think thinking 思考",
  "😱": "shock scared 震惊 害怕",
  "👋": "wave hi hello 挥手 你好",
  "🌈": "rainbow 彩虹",
  "❤️": "heart love 爱心 喜欢",
  "👏": "clap applause 鼓掌",
  "🐞": "bug ladybug 虫 瓢虫",
};

function cx(...values: Array<string | false | null | undefined>) {
  return values.filter(Boolean).join(" ");
}

export function ViceMeDanmaku({
  className,
  labels,
  maxLength = 40,
  messages,
  onRequestComposer,
  onSend,
}: DanmakuProps) {
  const reactId = useId();
  const stageRef = useRef<HTMLDivElement>(null);
  const seedIndex = useRef(0);
  const instanceCounter = useRef(0);
  const [laneSchedule] = useState(() =>
    Array.from({ length: DANMAKU_LANE_COUNT }, () => 0),
  );
  const [bullets, setBullets] = useState<ActiveMessage[]>([]);

  const removeBullet = useCallback((instanceId: string) => {
    setBullets((current) =>
      current.filter((item) => item.instanceId !== instanceId),
    );
  }, []);

  const spawn = useCallback(
    (message: DanmakuMessage) => {
      if (!stageRef.current) return;
      const text = normalizeDanmakuText(message.text, maxLength);
      if (!text) return;
      const reducedMotion = prefersReducedMotion();
      const reservation = reserveDanmakuLane({
        lanesBusyUntil: laneSchedule,
        bulletWidth: estimateDanmakuTextWidth(text),
        reducedMotion,
        now: performance.now(),
      });
      instanceCounter.current += 1;
      const bullet: ActiveMessage = {
        ...message,
        text,
        instanceId: `${reactId}-${message.id}-${instanceCounter.current}`,
        top: DANMAKU_LANE_TOP + reservation.lane * DANMAKU_LANE_HEIGHT,
        delayMs: reservation.delayMs,
        reducedMotion,
      };
      setBullets((current) => [...current.slice(-(MAX_VISIBLE - 1)), bullet]);
    },
    [laneSchedule, maxLength, reactId],
  );

  useEffect(() => {
    if (messages.length === 0) return;
    let cancelled = false;
    let timer = 0;
    const scheduleNext = (delay: number) => {
      timer = window.setTimeout(() => {
        if (cancelled) return;
        const message = messages[seedIndex.current % messages.length];
        seedIndex.current += 1;
        if (message) spawn(message);
        scheduleNext(SEED_INTERVAL_MS + 900 + Math.floor(Math.random() * 900));
      }, delay);
    };
    const frame = requestAnimationFrame(() => scheduleNext(120));
    return () => {
      cancelled = true;
      cancelAnimationFrame(frame);
      window.clearTimeout(timer);
    };
  }, [messages, spawn]);

  return (
    <div
      className={cx("pointer-events-none fixed inset-0 z-60", className)}
      data-slot="danmaku"
    >
      <div
        ref={stageRef}
        className="pointer-events-none absolute inset-x-0 top-0 h-40 overflow-hidden"
        aria-hidden
      >
        {bullets.map((bullet) => (
          <DanmakuBullet
            key={bullet.instanceId}
            bullet={bullet}
            onDone={removeBullet}
            stageRef={stageRef}
          />
        ))}
      </div>
      <DanmakuInteractionBar
        labels={labels}
        maxLength={maxLength}
        onRequestOpen={onRequestComposer}
        onSend={onSend}
        onSent={spawn}
      />
    </div>
  );
}

function DanmakuBullet({
  bullet,
  onDone,
  stageRef,
}: {
  bullet: ActiveMessage;
  onDone: (instanceId: string) => void;
  stageRef: RefObject<HTMLDivElement | null>;
}) {
  const bulletRef = useRef<HTMLSpanElement>(null);

  useLayoutEffect(() => {
    const element = bulletRef.current;
    const stage = stageRef.current;
    if (!element || !stage) return;
    return playDanmakuBullet({
      element,
      stage,
      text: bullet.text,
      delayMs: bullet.delayMs,
      reducedMotion: bullet.reducedMotion,
      onFinish: () => onDone(bullet.instanceId),
    });
  }, [bullet, onDone, stageRef]);

  return (
    <span
      ref={bulletRef}
      className={cx(
        "absolute inline-flex items-center whitespace-pre select-none font-normal leading-[1.125] text-white will-change-[transform,opacity,top,left]",
        bullet.self &&
          "rounded-sm border border-white/80 bg-white/15 px-1.5 py-px",
      )}
      style={{
        top: bullet.top,
        left: -10,
        opacity: 0,
        fontSize: DANMAKU_FONT_SIZE_PX,
        textShadow: "0 1px 2px rgb(0 0 0 / 0.45)",
        zIndex: bullet.self ? 2 : 1,
      }}
    >
      {bullet.text}
    </span>
  );
}

type PanelMode = "collapsed" | "greeting" | "reactions" | "typing" | "more";

function preventImeSubmit(event: KeyboardEvent<HTMLInputElement>) {
  if (
    event.key === "Enter" &&
    (event.nativeEvent.isComposing || event.keyCode === 229)
  ) {
    event.preventDefault();
  }
}

function DanmakuInteractionBar({
  labels,
  maxLength,
  onRequestOpen,
  onSend,
  onSent,
}: {
  labels: DanmakuLabels;
  maxLength: number;
  onRequestOpen: () => Promise<boolean>;
  onSend: (text: string) => Promise<DanmakuMessage | null>;
  onSent: (message: DanmakuMessage) => void;
}) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const collapseTimer = useRef(0);
  const greetingTimer = useRef(0);
  const permission = useRef<boolean | null>(null);
  const permissionRequest = useRef<Promise<boolean> | null>(null);
  const [mode, setMode] = useState<PanelMode>("greeting");
  const [greetingDismissed, setGreetingDismissed] = useState(false);
  const [sending, setSending] = useState(false);
  const [sendStatus, setSendStatus] = useState<"idle" | "error" | "success">(
    "idle",
  );
  const [emojiSearch, setEmojiSearch] = useState("");
  const [text, setText] = useState("");
  const expanded = mode !== "collapsed";
  const moreOpen = mode === "more";
  const { panelRef, present: morePresent } = useMoreReactionsPresence(moreOpen);
  const interactiveLayerOpen = mode === "typing" || morePresent;
  const query = emojiSearch.trim().toLocaleLowerCase();
  const filteredReactions = query
    ? MORE_REACTIONS.filter(
        (emoji) => emoji.includes(query) || SEARCH_TERMS[emoji].includes(query),
      )
    : MORE_REACTIONS;

  const clearCollapse = useCallback(() => {
    window.clearTimeout(collapseTimer.current);
  }, []);
  const scheduleCollapse = useCallback(() => {
    clearCollapse();
    collapseTimer.current = window.setTimeout(() => {
      setMode((current) => (current === "reactions" ? "collapsed" : current));
    }, AUTO_COLLAPSE_MS);
  }, [clearCollapse]);
  const dismissGreeting = useCallback(() => {
    window.clearTimeout(greetingTimer.current);
    setGreetingDismissed(true);
    setMode((current) => (current === "greeting" ? "reactions" : current));
  }, []);

  useEffect(() => {
    if (mode !== "greeting" || greetingDismissed) return;
    greetingTimer.current = window.setTimeout(
      dismissGreeting,
      GREETING_AUTO_DISMISS_MS,
    );
    return () => window.clearTimeout(greetingTimer.current);
  }, [dismissGreeting, greetingDismissed, mode]);
  useEffect(() => {
    if (mode === "reactions") scheduleCollapse();
    else clearCollapse();
    return clearCollapse;
  }, [clearCollapse, mode, scheduleCollapse]);
  useEffect(() => {
    if (mode === "typing") queueMicrotask(() => inputRef.current?.focus());
  }, [mode]);

  const ensureAllowed = useCallback(async () => {
    if (permission.current === true) return true;
    if (permissionRequest.current) return permissionRequest.current;
    const request = onRequestOpen()
      .then((allowed) => {
        permission.current = allowed;
        return allowed;
      })
      .finally(() => {
        permissionRequest.current = null;
      });
    permissionRequest.current = request;
    return request;
  }, [onRequestOpen]);

  const sendValue = useCallback(
    async (value: string) => {
      const normalized = normalizeDanmakuText(value, maxLength);
      if (!normalized) return false;
      setSendStatus("idle");
      try {
        if (!(await ensureAllowed())) return false;
        const message = await onSend(normalized);
        if (!message) return false;
        onSent({ ...message, self: true });
        setSendStatus("success");
        return true;
      } catch {
        setSendStatus("error");
        return false;
      }
    },
    [ensureAllowed, maxLength, onSend, onSent],
  );

  const sendReaction = useCallback(
    (emoji: string) => {
      clearCollapse();
      window.clearTimeout(greetingTimer.current);
      setGreetingDismissed(true);
      setMode("reactions");
      void sendValue(emoji).finally(scheduleCollapse);
    },
    [clearCollapse, scheduleCollapse, sendValue],
  );

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (sending) return;
    const value = normalizeDanmakuText(text, maxLength);
    if (!value) return;
    setSending(true);
    void sendValue(value)
      .then((sent) => {
        if (!sent) return;
        setText("");
        setMode("reactions");
      })
      .finally(() => setSending(false));
  };

  return (
    <>
      {interactiveLayerOpen ? (
        <button
          type="button"
          className="pointer-events-auto fixed inset-0 z-0 cursor-default bg-transparent outline-none"
          aria-label={labels.closeInteractiveLayer}
          onClick={() => !sending && setMode("reactions")}
        />
      ) : null}
      <div
        className={cx(
          "pointer-events-auto fixed left-1/2 z-1 flex -translate-x-1/2 items-start overflow-visible bg-[#1f1f21] text-sm text-[#cecfd2]",
          expanded
            ? "transition-[width,height,border-radius,bottom] duration-350 ease-[cubic-bezier(0.34,1.25,0.64,1)] motion-reduce:transition-none"
            : "transition-[width,height,border-radius,bottom] duration-250 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none",
          expanded
            ? "bottom-0 h-[calc(56px+env(safe-area-inset-bottom,0))] w-full rounded-none"
            : "bottom-[calc(12px+env(safe-area-inset-bottom,0))] size-8 rounded-md",
        )}
        data-state={mode}
        onPointerMove={() => mode === "reactions" && scheduleCollapse()}
      >
        {mode === "collapsed" ? (
          <IconButton
            label={labels.expandBar}
            onClick={() =>
              setMode(greetingDismissed ? "reactions" : "greeting")
            }
          >
            <LuSmilePlus size={20} aria-hidden />
          </IconButton>
        ) : (
          <div className="relative h-14 w-full">
            <button
              type="button"
              disabled={sending}
              className="absolute top-1/2 left-4 z-1 inline-flex size-8 -translate-y-1/2 items-center justify-center rounded-md text-white outline-none transition-[background-color] duration-150 ease-[cubic-bezier(0.22,1,0.36,1)] hover:bg-white/12 focus-visible:bg-white/12 disabled:opacity-40 motion-reduce:transition-none"
              aria-label={labels.collapseBar}
              onClick={() => setMode("collapsed")}
            >
              <LuChevronDown size={20} aria-hidden />
            </button>
            <div className="absolute top-1/2 left-1/2 max-w-[calc(100%-6rem)] -translate-x-1/2 -translate-y-1/2 min-[744px]:max-w-91.5">
              {mode === "typing" ? (
                <form className="flex h-8 w-full min-w-0" onSubmit={submit}>
                  <label className="sr-only" htmlFor={inputId}>
                    {labels.enterToSend}
                  </label>
                  <input
                    ref={inputRef}
                    id={inputId}
                    type="text"
                    inputMode="text"
                    enterKeyHint="send"
                    autoComplete="off"
                    value={text}
                    disabled={sending}
                    placeholder={labels.enterToSend}
                    aria-invalid={sendStatus === "error"}
                    className="h-8 min-w-0 flex-1 rounded-md border-0 bg-[#2b2b2e] px-3 text-base leading-5 text-[#cecfd2] shadow-[inset_0_0_0_1px_#7e8188] outline-none placeholder:text-[#a9aaad] focus-visible:shadow-[inset_0_0_0_1px_#cecfd2] disabled:opacity-55 min-[744px]:text-sm"
                    onChange={(event) =>
                      setText(
                        truncateDanmakuText(event.target.value, maxLength),
                      )
                    }
                    onKeyDown={preventImeSubmit}
                  />
                </form>
              ) : mode === "greeting" ? (
                <GreetingPrompt
                  label={labels.sayHi}
                  onClick={() => sendReaction("👋")}
                />
              ) : (
                <div className="flex items-center justify-center gap-2 min-[744px]:gap-4">
                  {QUICK_REACTIONS.map((emoji, index) => (
                    <ReactionButton
                      key={emoji}
                      emoji={emoji}
                      hiddenOnNarrow={index > 2}
                      label={labels.sendReaction(emoji)}
                      onSend={sendReaction}
                    />
                  ))}
                  <IconButton
                    label={labels.openComposer}
                    onClick={() => setMode("typing")}
                    largeIcon
                  >
                    <LuMessageCircleMore aria-hidden />
                  </IconButton>
                  <IconButton
                    label={labels.moreReactions}
                    ariaExpanded={moreOpen}
                    onClick={() =>
                      setMode((current) =>
                        current === "more" ? "reactions" : "more",
                      )
                    }
                    largeIcon
                  >
                    <LuSmilePlus aria-hidden />
                  </IconButton>
                </div>
              )}
            </div>
            {morePresent ? (
              <div
                ref={panelRef}
                className="absolute right-4 bottom-[calc(100%+12px)] h-68 w-[min(352px,calc(100vw-32px))] origin-bottom-right rounded-xl border border-[rgb(227_228_242/0.12)] bg-[#2b2c2f] shadow-[0_6px_24px_rgb(0_0_0/0.1)] will-change-[transform,opacity] min-[744px]:right-auto min-[744px]:left-1/2 min-[744px]:origin-bottom min-[744px]:-translate-x-1/2"
                role="group"
                aria-label={labels.moreReactions}
              >
                <div className="flex h-15 items-start gap-2 px-4 pt-4">
                  <div className="relative h-9 min-w-0 flex-1">
                    <LuSearch
                      className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-[#7e8188]"
                      size={18}
                      aria-hidden
                    />
                    <input
                      type="search"
                      value={emojiSearch}
                      placeholder={labels.searchEmoji}
                      aria-label={labels.searchEmoji}
                      className="size-full rounded-lg border-0 bg-[#242528] pr-3 pl-11 text-sm text-[#cecfd2] shadow-[inset_0_0_0_1px_#7e8188] outline-none placeholder:text-[#7e8188] focus-visible:shadow-[inset_0_0_0_2px_#cecfd2]"
                      onChange={(event) => setEmojiSearch(event.target.value)}
                    />
                  </div>
                  <ReactionButton
                    emoji="👋"
                    label={labels.sendReaction("👋")}
                    onSend={sendReaction}
                    popover
                  />
                </div>
                <p className="mt-3 px-4 text-xs font-medium text-[#cecfd2]">
                  {labels.frequentlyUsed}
                </p>
                <div className="mt-2.5 grid grid-cols-9 gap-1 px-4 pb-4">
                  {filteredReactions.map((emoji) => (
                    <ReactionButton
                      key={emoji}
                      emoji={emoji}
                      label={labels.sendReaction(emoji)}
                      onSend={sendReaction}
                      popover
                    />
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        )}
      </div>
      <p className="sr-only" role="status" aria-live="polite">
        {sendStatus === "success"
          ? labels.sent
          : sendStatus === "error"
            ? labels.submitFailed
            : ""}
      </p>
    </>
  );
}

function IconButton({
  ariaExpanded,
  children,
  label,
  largeIcon = false,
  onClick,
}: {
  ariaExpanded?: boolean;
  children: React.ReactNode;
  label: string;
  largeIcon?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={cx(
        "inline-flex size-8 shrink-0 items-center justify-center rounded-md p-0 leading-none text-white outline-none transition-[background-color] duration-150 ease-[cubic-bezier(0.22,1,0.36,1)] hover:bg-white/12 focus-visible:bg-white/12 active:bg-white/16 motion-reduce:transition-none",
        largeIcon && "[&_svg]:size-6 [&_svg]:shrink-0",
      )}
      aria-label={label}
      aria-expanded={ariaExpanded}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function GreetingPrompt({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  const progressRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const progress = progressRef.current;
    if (!progress) return;
    if (prefersReducedMotion() || !progress.animate) {
      progress.style.transform = "scaleX(1)";
      return;
    }
    const animation = progress.animate(
      [{ transform: "scaleX(0.5)" }, { transform: "scaleX(1)" }],
      {
        duration: GREETING_AUTO_DISMISS_MS,
        fill: "forwards",
        easing: "linear",
      },
    );
    return () => animation.cancel();
  }, []);

  return (
    <button
      type="button"
      className="relative inline-flex h-9 w-full max-w-91.5 items-center justify-center overflow-hidden rounded-full border border-[rgb(227_228_242/0.14)] bg-[#2b2b2e] px-4 text-sm font-medium text-[#cecfd2] outline-none transition-[background-color] duration-150 ease-[cubic-bezier(0.22,1,0.36,1)] hover:bg-[#38383c] focus-visible:bg-[#38383c] active:bg-[#424247] motion-reduce:transition-none"
      aria-label={label}
      onClick={onClick}
    >
      <span
        ref={progressRef}
        aria-hidden
        className="pointer-events-none absolute inset-y-0 right-0 w-full origin-right bg-[rgb(0_0_0/0.28)]"
        style={{ transform: "scaleX(0.5)" }}
      />
      <span className="relative z-1">{label}</span>
    </button>
  );
}

function ReactionButton({
  emoji,
  hiddenOnNarrow = false,
  label,
  onSend,
  popover = false,
}: {
  emoji: string;
  hiddenOnNarrow?: boolean;
  label: string;
  onSend: (emoji: string) => void;
  popover?: boolean;
}) {
  const delayTimer = useRef(0);
  const repeatTimer = useRef(0);
  const repeated = useRef(false);
  const stopRepeat = useCallback(() => {
    window.clearTimeout(delayTimer.current);
    window.clearInterval(repeatTimer.current);
  }, []);
  useEffect(() => stopRepeat, [stopRepeat]);

  const pointerDown = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    repeated.current = false;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    delayTimer.current = window.setTimeout(() => {
      repeated.current = true;
      onSend(emoji);
      repeatTimer.current = window.setInterval(
        () => onSend(emoji),
        LONG_PRESS_REPEAT_MS,
      );
    }, LONG_PRESS_DELAY_MS);
  };

  return (
    <button
      type="button"
      className={cx(
        "shrink-0 touch-none select-none items-center justify-center leading-none outline-none",
        popover
          ? "inline-flex size-8 rounded-lg text-[22px] transition-[background-color] duration-150 ease-[cubic-bezier(0.22,1,0.36,1)] hover:bg-[rgb(206_207_210/0.12)] focus-visible:bg-[rgb(206_207_210/0.12)] active:bg-[rgb(206_207_210/0.16)] motion-reduce:transition-none"
          : "group inline-flex size-8 overflow-visible rounded-md text-2xl",
        hiddenOnNarrow && "hidden min-[440px]:inline-flex",
      )}
      aria-label={label}
      onClick={() => {
        if (repeated.current) {
          repeated.current = false;
          return;
        }
        onSend(emoji);
      }}
      onContextMenu={(event) => event.preventDefault()}
      onPointerDown={pointerDown}
      onPointerUp={stopRepeat}
      onPointerCancel={stopRepeat}
      onLostPointerCapture={stopRepeat}
    >
      {popover ? (
        emoji
      ) : (
        <span
          aria-hidden
          className="pointer-events-none block origin-bottom transition-transform duration-320 ease-[cubic-bezier(0.34,3.85,0.64,1)] group-hover:translate-y-[-7.8px] group-hover:scale-[1.3] group-hover:duration-250 group-hover:ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none motion-reduce:group-hover:transform-none"
        >
          {emoji}
        </span>
      )}
    </button>
  );
}
