// @vitest-environment jsdom

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { afterEach, describe, expect, it, vi } from "vitest";

const loaderSource = readFileSync(
  join(process.cwd(), ".generated/cdn/viceme-danmaku-widget.js"),
  "utf8",
);

declare global {
  interface Window {
    ViceMeDanmakuWidget?: {
      init: () => Element[];
      version: string;
    };
  }
}

function runLoader() {
  window.eval(loaderSource);
}

function appendEmbed(attrs: Record<string, string>) {
  const script = document.createElement("script");
  if (attrs["data-name"] !== "omit") {
    script.setAttribute("data-name", attrs["data-name"] ?? "ViceMe-Danmaku");
  }
  script.src = attrs.src ?? "https://cdn.viceme.cn/danmaku/v1/widget.js";
  for (const [key, value] of Object.entries(attrs)) {
    if (key !== "src" && !(key === "data-name" && value === "omit")) {
      script.setAttribute(key, value);
    }
  }
  document.body.appendChild(script);
  return script;
}

afterEach(() => {
  document.body.innerHTML = "";
  delete window.ViceMeDanmakuWidget;
  vi.restoreAllMocks();
});

describe("ViceMe hosted danmaku loader", () => {
  it("mounts the stage, controls, and lazy modal frames from a four-line script", () => {
    const script = appendEmbed({
      "data-creator-id": "creator_123",
      "data-work-id": "work_456",
      "data-theme": "dark",
    });

    runLoader();

    const root = document.querySelector("[data-viceme-danmaku='mounted']");
    expect(root?.id).toBe("viceme-danmaku-root-creator_123-work_456");
    expect(script.getAttribute("data-viceme-mounted")).toBe("true");

    const frames = Array.from(root!.querySelectorAll("iframe"));
    expect(frames).toHaveLength(3);
    expect(frames[0].title).toBe("ViceMe Danmaku");
    expect(frames[0].style.pointerEvents).toBe("none");
    expect(frames[0].src).toContain("https://viceme.cn/embed/danmaku?");
    expect(frames[0].src).toContain("mode=stage");
    expect(frames[0].src).toContain("creatorId=creator_123");
    expect(frames[0].src).toContain("workId=work_456");
    expect(frames[0].src).toContain("theme=dark");

    expect(frames[1].title).toBe("ViceMe Danmaku controls");
    expect(frames[1].style.pointerEvents).toBe("auto");
    expect(frames[1].src).toContain("mode=controls");

    expect(frames[2].title).toBe("ViceMe Danmaku dialog");
    expect(frames[2].src).toBe("about:blank");
    expect(frames[2].getAttribute("data-src")).toContain("mode=modal");
    expect(frames[2].style.display).toBe("none");
  });

  it("uses the global widget origin for global CDN snippets", () => {
    appendEmbed({
      src: "https://cdn.viceme.ai/danmaku/v1/widget.js",
      "data-creator-id": "creator",
      "data-work-id": "work",
    });

    runLoader();

    const stage = document.querySelector("iframe")!;
    expect(stage.src).toContain("https://viceme.ai/embed/danmaku?");
  });

  it("uses the script origin for local four-line demos", () => {
    appendEmbed({
      src: "http://localhost:4300/danmaku/v1/widget.js",
      "data-creator-id": "creator",
      "data-work-id": "work",
    });

    runLoader();

    const stage = document.querySelector("iframe")!;
    expect(stage.src).toContain("http://localhost:4300/embed/danmaku?");
  });

  it("supports the documented four-line snippet without data-name", () => {
    appendEmbed({
      "data-name": "omit",
      "data-creator-id": "creator",
      "data-work-id": "work",
    });

    runLoader();

    expect(document.querySelector("[data-viceme-danmaku='mounted']")).toBeTruthy();
  });

  it("does not mount twice for the same creator and work", () => {
    appendEmbed({
      "data-creator-id": "creator",
      "data-work-id": "work",
    });

    runLoader();
    window.ViceMeDanmakuWidget!.init();

    expect(document.querySelectorAll("[data-viceme-danmaku='mounted']")).toHaveLength(
      1,
    );
  });

  it("fails closed when required binding attributes are missing", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    appendEmbed({ "data-creator-id": "creator" });

    runLoader();

    expect(warn).toHaveBeenCalledWith(
      expect.stringContaining("missing data-creator-id or data-work-id"),
      expect.any(HTMLScriptElement),
    );
    expect(document.querySelector("[data-viceme-danmaku='mounted']")).toBeNull();
  });
});
