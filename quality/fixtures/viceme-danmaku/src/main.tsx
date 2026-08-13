import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import {
  ViceMeDanmaku,
  type DanmakuLabels,
} from "../.generated/danmaku-blueprint";

import "./styles.css";

const labels: DanmakuLabels = {
  closeInteractiveLayer: "Close interactive layer",
  collapseBar: "Collapse danmaku",
  enterToSend: "Enter a danmaku",
  expandBar: "Expand danmaku",
  frequentlyUsed: "Frequently used",
  moreReactions: "More reactions",
  openComposer: "Open composer",
  sayHi: "Say hi",
  searchEmoji: "Search emoji",
  sendReaction: (emoji) => `Send ${emoji}`,
  sent: "Sent",
  submitFailed: "Submit failed",
};

const root = document.getElementById("root");
if (!root) throw new Error("fixture root is missing");

createRoot(root).render(
  <StrictMode>
    <main className="min-h-screen bg-slate-950 text-white">
      <h1 className="p-8 text-2xl">ViceMe Danmaku Fixture</h1>
      <ViceMeDanmaku
        labels={labels}
        messages={[{ id: "fixture", text: "Golden component fixture" }]}
        onRequestComposer={async () => true}
        onSend={async (text) => ({ id: `fixture-${text}`, text })}
      />
    </main>
  </StrictMode>,
);
