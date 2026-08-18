// Adapt imports and assertions to the target repository's test runner.
// @vitest-environment jsdom

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ViceMeDanmaku,
  type DanmakuLabels,
  type DanmakuMessage,
  type DanmakuProps,
} from "./danmaku-blueprint";
import {
  DANMAKU_STATIC_DURATION_MS,
  DANMAKU_STATIC_LEFT_PX,
  playDanmakuBullet,
  randomReactionHoverTiltDeg,
  reactionHoverTransform,
} from "./danmaku-motion";

const labels: DanmakuLabels = {
  closeInteractiveLayer: "close-interactive-layer",
  collapseBar: "collapse-bar",
  enterToSend: "enter-to-send",
  expandBar: "expand-bar",
  frequentlyUsed: "frequently-used",
  moreReactions: "more-reactions",
  openComposer: "open-composer",
  sayHi: "say-hi",
  searchEmoji: "search-emoji",
  sendReaction: (emoji) => `send-${emoji}`,
  sent: "sent",
  submitFailed: "submit-failed",
};

type RenderOptions = {
  allowed?: boolean;
  messages?: DanmakuMessage[];
  onRequestComposer?: DanmakuProps["onRequestComposer"];
  onSend?: DanmakuProps["onSend"];
};

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

function renderDanmaku({
  allowed = true,
  messages = [],
  onRequestComposer = vi.fn(async () => allowed),
  onSend = vi.fn(async (text: string) => ({ id: `sent-${text}`, text })),
}: RenderOptions = {}) {
  const result = render(
    <ViceMeDanmaku
      labels={labels}
      messages={messages}
      onRequestComposer={onRequestComposer}
      onSend={onSend}
    />,
  );
  return { ...result, onRequestComposer, onSend };
}

function interactionBar(container: HTMLElement) {
  const bar = container.querySelector<HTMLElement>("[data-state]");
  if (!bar) throw new Error("interaction bar was not rendered");
  return bar;
}

function advanceGreeting() {
  act(() => vi.advanceTimersByTime(3500));
}

async function sendGreeting(onSend: DanmakuProps["onSend"]) {
  fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
  await waitFor(() => expect(onSend).toHaveBeenCalledWith("👋"));
}

describe("ViceMeDanmaku golden behavior", () => {
  it("auto-dismisses the first-use greeting after 3500ms", () => {
    vi.useFakeTimers();
    renderDanmaku();
    expect(screen.getByRole("button", { name: labels.sayHi })).toBeTruthy();

    advanceGreeting();

    expect(screen.queryByRole("button", { name: labels.sayHi })).toBeNull();
    expect(screen.getByRole("button", { name: "send-❤️" })).toBeTruthy();
  });

  it("raises a full-width Dock bar and keeps the launcher bottom-centered", () => {
    vi.useFakeTimers();
    const { container } = renderDanmaku();
    const bar = interactionBar(container);
    expect(bar.className).toContain("inset-x-0");
    expect(bar.className).toContain("w-full");
    expect(bar.className).not.toContain("transition-[width");

    const openMotion = vi
      .mocked(Element.prototype.animate)
      .mock.calls.find(([frames]) => {
        const first = Array.isArray(frames) ? frames[0] : undefined;
        return first?.transform === "translateY(calc(100% + 8px))";
      });
    expect(openMotion?.[0]).toEqual([
      { transform: "translateY(calc(100% + 8px))" },
      { transform: "translateY(0)" },
    ]);
    expect(openMotion?.[1]).toMatchObject({
      duration: 350,
      easing: "cubic-bezier(0.22, 1, 0.36, 1)",
    });

    advanceGreeting();
    const heart = screen.getByRole("button", { name: "send-❤️" });
    const centerAnchor = heart.closest(".group")?.parentElement?.parentElement;
    expect(centerAnchor?.className).toContain("left-1/2");
    expect(centerAnchor?.className).toContain("-translate-x-1/2");

    for (const emoji of ["👏", "🙌", "👀"]) {
      const secondary = screen.getByRole("button", { name: `send-${emoji}` });
      const secondaryGroup = secondary.closest(".group");
      expect(secondaryGroup?.className).toContain("hidden");
      expect(secondaryGroup?.className).toContain("min-[440px]:inline-flex");
    }
    expect(
      screen.getByRole("button", { name: labels.openComposer }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: labels.moreReactions }),
    ).toBeTruthy();

    act(() => vi.advanceTimersByTime(14999));
    expect(bar.dataset.state).toBe("reactions");
    act(() => vi.advanceTimersByTime(1));
    expect(bar.dataset.state).toBe("collapsed");
    expect(bar.style.transform).toBe("translateY(calc(100% + 8px))");
    const launcher = screen.getByRole("button", {
      name: labels.expandBar,
    }).parentElement?.parentElement;
    expect(launcher?.className).toContain("left-1/2");
    expect(launcher?.className).toContain("-translate-x-1/2");
    expect(launcher?.className).toContain(
      "bottom-[calc(12px+env(safe-area-inset-bottom,0))]",
    );
  });

  it("sends the greeting and reuses the successful permission", async () => {
    const onSend = vi.fn<DanmakuProps["onSend"]>(async (text) => ({
      id: `sent-${text}`,
      text,
    }));
    const { onRequestComposer } = renderDanmaku({ onSend });
    await sendGreeting(onSend);
    fireEvent.click(screen.getByRole("button", { name: "send-❤️" }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("❤️"));
    expect(onSend.mock.calls.filter(([text]) => text === "❤️")).toHaveLength(1);
    expect(onRequestComposer).toHaveBeenCalledTimes(1);
  });

  it("does not send when the host denies permission", async () => {
    const { onSend } = renderDanmaku({ allowed: false });
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => Promise.resolve());
    expect(onSend).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("catches authorization rejection, reports failure, and allows retry", async () => {
    const onRequestComposer = vi
      .fn<DanmakuProps["onRequestComposer"]>()
      .mockRejectedValueOnce(new Error("login unavailable"))
      .mockResolvedValueOnce(true);
    const onSend = vi.fn<DanmakuProps["onSend"]>(async (text) => ({
      id: `sent-${text}`,
      text,
    }));
    renderDanmaku({ onRequestComposer, onSend });

    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe(labels.submitFailed),
    );
    expect(onSend).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "send-❤️" }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("❤️"));
    expect(onRequestComposer).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("status").textContent).toBe(labels.sent);
  });

  it("keeps the composer recoverable when authorization rejects", async () => {
    vi.useFakeTimers();
    const onRequestComposer = vi
      .fn<DanmakuProps["onRequestComposer"]>()
      .mockRejectedValueOnce(new Error("session lookup failed"))
      .mockResolvedValueOnce(true);
    const onSend = vi.fn<DanmakuProps["onSend"]>(async (text) => ({
      id: `sent-${text}`,
      text,
    }));
    renderDanmaku({ onRequestComposer, onSend });
    advanceGreeting();
    fireEvent.click(screen.getByRole("button", { name: labels.openComposer }));
    const input = screen.getByRole("textbox", {
      name: labels.enterToSend,
    }) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "retry me" } });

    fireEvent.submit(input.closest("form")!);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByRole("status").textContent).toBe(labels.submitFailed);
    expect(input.value).toBe("retry me");
    expect(input.disabled).toBe(false);
    expect(onSend).not.toHaveBeenCalled();

    fireEvent.submit(input.closest("form")!);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(onSend).toHaveBeenCalledWith("retry me");
    expect(onRequestComposer).toHaveBeenCalledTimes(2);
  });

  it("opens a focused Enter-to-send input and suppresses IME submission", async () => {
    const { onSend } = renderDanmaku();
    await sendGreeting(onSend);
    fireEvent.click(screen.getByRole("button", { name: labels.openComposer }));
    const input = screen.getByRole("textbox", { name: labels.enterToSend });
    await waitFor(() => expect(document.activeElement).toBe(input));
    fireEvent.change(input, { target: { value: "做得很棒" } });
    fireEvent.keyDown(input, {
      key: "Enter",
      keyCode: 229,
      isComposing: true,
    });
    expect(onSend).not.toHaveBeenCalledWith("做得很棒");

    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("做得很棒"));
  });

  it("keeps failed text recoverable and emits a self bullet only after success", async () => {
    const onSend = vi
      .fn<DanmakuProps["onSend"]>()
      .mockResolvedValueOnce({ id: "wave", text: "👋" })
      .mockRejectedValueOnce(new Error("network unavailable"))
      .mockResolvedValueOnce({ id: "recovered", text: "恢复成功" });
    const { onRequestComposer } = renderDanmaku({ onSend });
    await sendGreeting(onSend);
    fireEvent.click(screen.getByRole("button", { name: labels.openComposer }));
    const input = screen.getByRole("textbox", {
      name: labels.enterToSend,
    }) as HTMLInputElement;

    fireEvent.change(input, { target: { value: "保留这条" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe(labels.submitFailed),
    );
    expect(input.value).toBe("保留这条");
    expect(input.disabled).toBe(false);
    expect(screen.queryByText("保留这条")).toBeNull();

    fireEvent.change(input, { target: { value: "恢复成功" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe(labels.sent),
    );
    expect(
      screen.queryByRole("textbox", { name: labels.enterToSend }),
    ).toBeNull();
    const selfBullet = screen.getByText("恢复成功");
    expect(selfBullet.className).toContain("border-white/80");
    expect(onRequestComposer).toHaveBeenCalledTimes(1);
  });

  it("opens, filters, toggles, and outside-closes the mounted popover", () => {
    vi.useFakeTimers();
    renderDanmaku();
    advanceGreeting();
    const more = screen.getByRole("button", { name: labels.moreReactions });

    fireEvent.click(more);
    const palette = within(
      screen.getByRole("group", { name: labels.moreReactions }),
    );
    const search = palette.getByRole("searchbox", {
      name: labels.searchEmoji,
    });
    fireEvent.change(search, { target: { value: "rainbow" } });
    expect(palette.getByRole("button", { name: "send-🌈" })).toBeTruthy();
    expect(palette.queryByRole("button", { name: "send-❤️" })).toBeNull();

    fireEvent.click(more);
    expect(more.getAttribute("aria-expanded")).toBe("false");
    expect(
      screen.getByRole("group", { name: labels.moreReactions }),
    ).toBeTruthy();
    act(() => vi.advanceTimersByTime(149));
    expect(
      screen.getByRole("group", { name: labels.moreReactions }),
    ).toBeTruthy();
    act(() => vi.advanceTimersByTime(1));
    expect(
      screen.queryByRole("group", { name: labels.moreReactions }),
    ).toBeNull();

    fireEvent.click(more);
    expect(
      screen.getByRole("group", { name: labels.moreReactions }),
    ).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: labels.closeInteractiveLayer }),
    );
    expect(
      screen.getByRole("group", { name: labels.moreReactions }),
    ).toBeTruthy();
    act(() => vi.advanceTimersByTime(150));
    expect(
      screen.queryByRole("group", { name: labels.moreReactions }),
    ).toBeNull();
  });

  it("repeats a held quick reaction and suppresses its trailing click", async () => {
    vi.useFakeTimers();
    const onSend = vi.fn<DanmakuProps["onSend"]>(async (text) => ({
      id: `sent-${text}`,
      text,
    }));
    renderDanmaku({ onSend });
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const fire = screen.getByRole("button", { name: "send-🔥" });
    fireEvent.pointerDown(fire, {
      button: 0,
      pointerId: 1,
      pointerType: "touch",
    });
    await act(async () => {
      vi.advanceTimersByTime(860);
      await Promise.resolve();
    });
    fireEvent.pointerUp(fire, { pointerId: 1, pointerType: "touch" });
    fireEvent.click(fire);

    expect(onSend.mock.calls.filter(([text]) => text === "🔥")).toHaveLength(3);
  });

  it("renders Loom-style semantic tooltip markup on quick reactions", async () => {
    vi.useFakeTimers();
    renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const heart = screen.getByRole("button", { name: "send-❤️" });
    const group = heart.closest(".group");
    expect(group).toBeTruthy();
    const tooltipRoot = group?.querySelector("[aria-hidden]");
    expect(tooltipRoot?.textContent).toBe("heart");
    expect(group?.contains(heart)).toBe(true);
    expect(tooltipRoot && group?.contains(tooltipRoot)).toBe(true);

    const comment = screen.getByRole("button", { name: labels.openComposer });
    expect(comment.closest(".group")?.querySelector("[aria-hidden]")?.textContent).toBe(
      "comment",
    );
    const more = screen.getByRole("button", { name: labels.moreReactions });
    expect(more.closest(".group")?.querySelector("[aria-hidden]")?.textContent).toBe(
      "more reactions",
    );
  });

  it("applies a single Loom hover transform on the wrapper that owns the tooltip", async () => {
    vi.useFakeTimers();
    renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const fire = screen.getByRole("button", { name: "send-🔥" });
    const wrap = fire.closest(".group");
    expect(wrap).toBeTruthy();
    fireEvent.pointerEnter(wrap as HTMLElement);
    expect((wrap as HTMLElement).style.transform).toMatch(
      /^scale\(1\.3\) translate\(0px, -6px\) rotate\(-?\d+deg\)$/,
    );
    const tilt = Number(
      /rotate\((-?\d+)deg\)/.exec((wrap as HTMLElement).style.transform)?.[1],
    );
    expect(tilt).toBeGreaterThanOrEqual(-5);
    expect(tilt).toBeLessThanOrEqual(5);
    fireEvent.pointerLeave(wrap as HTMLElement);
    expect((wrap as HTMLElement).style.transform).toBe("");

    const commentWrap = screen
      .getByRole("button", { name: labels.openComposer })
      .closest(".group");
    fireEvent.pointerEnter(commentWrap as HTMLElement);
    expect((commentWrap as HTMLElement).style.transform).toMatch(
      /^scale\(1\.3\) translate\(0px, -6px\) rotate\(-?\d+deg\)$/,
    );
    fireEvent.pointerLeave(commentWrap as HTMLElement);
    expect((commentWrap as HTMLElement).style.transform).toBe("");
  });

  it("maps hover tilt samples onto the Loom transform string", () => {
    expect(randomReactionHoverTiltDeg(() => 0)).toBe(-5);
    expect(randomReactionHoverTiltDeg(() => 0.999)).toBe(5);
    expect(reactionHoverTransform(4)).toBe(
      "scale(1.3) translate(0px, -6px) rotate(4deg)",
    );
  });

  it("auto-collapses reactions after 15000ms and expands without greeting", async () => {
    vi.useFakeTimers();
    const { container, onSend } = renderDanmaku();
    const bar = interactionBar(container);
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(onSend).toHaveBeenCalledWith("👋");
    act(() => vi.advanceTimersByTime(14999));
    expect(bar.dataset.state).toBe("reactions");
    act(() => vi.advanceTimersByTime(1));
    expect(bar.dataset.state).toBe("collapsed");
    fireEvent.click(screen.getByRole("button", { name: labels.expandBar }));
    expect(screen.queryByRole("button", { name: labels.sayHi })).toBeNull();
    expect(screen.getByRole("button", { name: "send-❤️" })).toBeTruthy();
  });

  it("uses static reduced-motion bullets and cancels their finish timer", () => {
    vi.useFakeTimers();
    const element = document.createElement("span");
    const stage = document.createElement("div");
    const onFinish = vi.fn();
    const stop = playDanmakuBullet({
      element,
      stage,
      text: "static",
      delayMs: 0,
      reducedMotion: true,
      onFinish,
    });

    expect(element.style.left).toBe(`${DANMAKU_STATIC_LEFT_PX}px`);
    expect(element.style.opacity).toBe("0.8");
    act(() => vi.advanceTimersByTime(DANMAKU_STATIC_DURATION_MS - 1));
    expect(onFinish).not.toHaveBeenCalled();
    stop();
    act(() => vi.advanceTimersByTime(1));
    expect(onFinish).not.toHaveBeenCalled();
  });

  it("cancels moving bullet animations during cleanup", () => {
    const element = document.createElement("span");
    const stage = document.createElement("div");
    const animation = {
      addEventListener: vi.fn(),
      cancel: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as Animation;
    vi.spyOn(element, "animate").mockReturnValue(animation);

    const stop = playDanmakuBullet({
      element,
      stage,
      text: "moving",
      delayMs: 0,
      reducedMotion: false,
      onFinish: vi.fn(),
    });
    expect(animation.addEventListener).toHaveBeenCalledWith(
      "finish",
      expect.any(Function),
    );

    stop();
    expect(animation.removeEventListener).toHaveBeenCalledWith(
      "finish",
      expect.any(Function),
    );
    expect(animation.cancel).toHaveBeenCalledTimes(1);
  });

  it("disables popover close motion when reduced motion is requested", () => {
    vi.useFakeTimers();
    vi.mocked(window.matchMedia).mockImplementation((query) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    renderDanmaku();
    advanceGreeting();
    const more = screen.getByRole("button", { name: labels.moreReactions });
    fireEvent.click(more);
    expect(
      screen.getByRole("group", { name: labels.moreReactions }),
    ).toBeTruthy();
    fireEvent.click(more);
    expect(
      screen.queryByRole("group", { name: labels.moreReactions }),
    ).toBeNull();
  });

  it("cleans component timers, frames, intervals, and animations on unmount", () => {
    vi.useFakeTimers();
    const clearTimeoutSpy = vi.spyOn(window, "clearTimeout");
    const clearIntervalSpy = vi.spyOn(window, "clearInterval");
    const cancelFrameSpy = vi.spyOn(window, "cancelAnimationFrame");
    const animateMock = vi.mocked(Element.prototype.animate);
    const { unmount } = renderDanmaku({
      messages: [{ id: "seed", text: "seed" }],
    });
    advanceGreeting();
    const heart = screen.getByRole("button", { name: "send-❤️" });
    fireEvent.pointerDown(heart, {
      button: 0,
      pointerId: 2,
      pointerType: "touch",
    });
    fireEvent.click(screen.getByRole("button", { name: labels.moreReactions }));
    act(() => vi.advanceTimersByTime(16));

    unmount();

    expect(clearTimeoutSpy).toHaveBeenCalled();
    expect(clearIntervalSpy).toHaveBeenCalled();
    expect(cancelFrameSpy).toHaveBeenCalled();
    expect(animateMock).toHaveBeenCalled();
    const firstAnimation = animateMock.mock.results[0]?.value;
    expect(firstAnimation?.cancel).toHaveBeenCalled();
  });
});
