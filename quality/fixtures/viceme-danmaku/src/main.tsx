import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";

import {
  ViceMeDanmaku,
  type DanmakuLabels,
} from "../.generated/danmaku-blueprint";

import "./styles.css";

const labels: DanmakuLabels = {
  closeInteractiveLayer: "关闭互动层",
  collapseBar: "收起弹幕",
  enterToSend: "发一条匿名弹幕",
  expandBar: "展开弹幕",
  frequentlyUsed: "常用",
  moreReactions: "更多反应",
  openComposer: "打开输入框",
  sayHi: "打个招呼",
  searchEmoji: "搜索 emoji",
  sendReaction: (emoji) => `发送 ${emoji}`,
  sent: "已发送",
  submitFailed: "发送失败",
};

const seedMessages = [
  { id: "seed-1", text: "欢迎来到 ViceMe 弹幕" },
  { id: "seed-2", text: "匿名也能先发一句" },
  { id: "seed-3", text: "这个样式来自 golden blueprint" },
  { id: "seed-4", text: "舞台层不挡宿主页点击" },
];

function EmbeddedDanmaku() {
  const mode =
    new URLSearchParams(window.location.search).get("mode") || "full";

  useEffect(() => {
    document.body.dataset.embedMode = mode;
    if (window.parent === window) return;

    const postHeight = (height: number) => {
      window.parent.postMessage(
        { source: "viceme-danmaku", action: "resize-controls", height },
        "*",
      );
    };

    if (mode !== "controls") return;
    const resizeFromState = () => {
      const state = document.querySelector<HTMLElement>("[data-state]")
        ?.dataset.state;
      postHeight(state === "more" ? 360 : 88);
    };
    resizeFromState();
    const observer = new MutationObserver(resizeFromState);
    const watch = () => {
      const target = document.querySelector("[data-state]");
      if (target) {
        observer.observe(target, {
          attributeFilter: ["data-state"],
          attributes: true,
        });
      }
    };
    const frame = requestAnimationFrame(watch);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, [mode]);

  if (mode === "modal") {
    return (
      <div className="grid min-h-screen place-items-center bg-black/45 p-6">
        <section className="w-full max-w-md rounded-lg bg-white p-6 text-slate-950 shadow-2xl">
          <h1 className="text-xl font-semibold">评论区 mock</h1>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            这里代表后续 hosted widget app 里的评论、关注和赞赏区域。
          </p>
          <button
            type="button"
            className="mt-5 h-9 rounded-md bg-slate-950 px-4 text-sm font-medium text-white"
            onClick={() =>
              window.parent.postMessage(
                { source: "viceme-danmaku", action: "close-modal" },
                "*",
              )
            }
          >
            关闭
          </button>
        </section>
      </div>
    );
  }

  return (
    <ViceMeDanmaku
      labels={labels}
      messages={seedMessages}
      onRequestComposer={async () => true}
      onSend={async (text) => ({
        id: `fixture-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        text,
      })}
    />
  );
}

const root = document.getElementById("root");
if (!root) throw new Error("fixture root is missing");

createRoot(root).render(
  <StrictMode>
    <EmbeddedDanmaku />
  </StrictMode>,
);
