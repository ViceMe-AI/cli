import { useLayoutEffect, useRef, useState } from "react";

import { prefersReducedMotion } from "./danmaku-motion";

export function useMoreReactionsPresence(open: boolean) {
  const [present, setPresent] = useState(open);
  const panelRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!present) {
      if (open) setPresent(true);
      return;
    }

    const reduced = prefersReducedMotion();
    if (!open && reduced) {
      setPresent(false);
      return;
    }

    const duration = reduced ? 0 : open ? 250 : 150;
    let animation: Animation | undefined;
    const frame = requestAnimationFrame(() => {
      if (!panelRef.current?.animate) return;
      animation = panelRef.current.animate(
        open
          ? [
              { opacity: 0, transform: "scale(0.97)" },
              { opacity: 1, transform: "scale(1)" },
            ]
          : [
              { opacity: 1, transform: "scale(1)" },
              { opacity: 0, transform: "scale(0.99)" },
            ],
        {
          duration,
          easing: "cubic-bezier(0.22, 1, 0.36, 1)",
          fill: "both",
        },
      );
    });
    const timer = open
      ? undefined
      : window.setTimeout(() => setPresent(false), duration);

    return () => {
      cancelAnimationFrame(frame);
      if (timer !== undefined) window.clearTimeout(timer);
      animation?.cancel();
    };
  }, [open, present]);

  return { panelRef, present };
}
