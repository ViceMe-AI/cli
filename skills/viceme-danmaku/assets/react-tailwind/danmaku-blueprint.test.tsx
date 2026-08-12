// Adapt imports and assertions to the target repository's test runner.
// @vitest-environment jsdom

import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ViceMeDanmaku, type DanmakuLabels } from "./danmaku-blueprint";

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

afterEach(() => {
  vi.useRealTimers();
});

function renderDanmaku(allowed = true) {
  const onRequestComposer = vi.fn(async () => allowed);
  const onSend = vi.fn(async (text: string) => ({ id: text, text }));
  render(
    <ViceMeDanmaku
      labels={labels}
      messages={[]}
      onRequestComposer={onRequestComposer}
      onSend={onSend}
    />,
  );
  return { onRequestComposer, onSend };
}

describe("ViceMeDanmaku golden behavior", () => {
  it("auto-dismisses the first-use greeting after 3500ms", () => {
    vi.useFakeTimers();
    renderDanmaku();
    expect(screen.getByRole("button", { name: labels.sayHi })).toBeTruthy();

    act(() => vi.advanceTimersByTime(3500));

    expect(screen.queryByRole("button", { name: labels.sayHi })).toBeNull();
    expect(screen.getByRole("button", { name: "send-❤️" })).toBeTruthy();
  });

  it("sends the greeting and reuses the successful permission", async () => {
    const { onRequestComposer, onSend } = renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("👋"));
    fireEvent.click(screen.getByRole("button", { name: "send-❤️" }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("❤️"));
    expect(onRequestComposer).toHaveBeenCalledTimes(1);
  });

  it("does not send when the host denies permission", async () => {
    const { onSend } = renderDanmaku(false);
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => Promise.resolve());
    expect(onSend).not.toHaveBeenCalled();
  });

  it("opens a focused Enter-to-send input and suppresses IME submission", async () => {
    const { onSend } = renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("👋"));
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

  it("opens and filters the more-reactions popover", async () => {
    const { onSend } = renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("👋"));
    fireEvent.click(screen.getByRole("button", { name: labels.moreReactions }));
    const palette = within(
      screen.getByRole("group", { name: labels.moreReactions }),
    );
    const search = palette.getByRole("searchbox", {
      name: labels.searchEmoji,
    });
    fireEvent.change(search, { target: { value: "rainbow" } });
    expect(palette.getByRole("button", { name: "send-🌈" })).toBeTruthy();
    expect(palette.queryByRole("button", { name: "send-❤️" })).toBeNull();
  });

  it("repeats a held quick reaction and suppresses its trailing click", async () => {
    vi.useFakeTimers();
    const { onSend } = renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => Promise.resolve());
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

  it("auto-collapses reactions after 4000ms and does not replay greeting", async () => {
    vi.useFakeTimers();
    const { onSend } = renderDanmaku();
    fireEvent.click(screen.getByRole("button", { name: labels.sayHi }));
    await act(async () => Promise.resolve());
    expect(onSend).toHaveBeenCalledWith("👋");
    act(() => vi.advanceTimersByTime(4000));
    fireEvent.click(screen.getByRole("button", { name: labels.expandBar }));
    expect(screen.queryByRole("button", { name: labels.sayHi })).toBeNull();
    expect(screen.getByRole("button", { name: "send-❤️" })).toBeTruthy();
  });
});
