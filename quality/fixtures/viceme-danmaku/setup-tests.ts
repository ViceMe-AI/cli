import { beforeEach, vi } from "vitest";

function mediaQueryList(query: string, matches = false): MediaQueryList {
  return {
    matches,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(() => false),
  };
}

function animationStub(): Animation {
  return {
    addEventListener: vi.fn(),
    cancel: vi.fn(),
    removeEventListener: vi.fn(),
  } as unknown as Animation;
}

beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn((query: string) => mediaQueryList(query)),
  });
  Object.defineProperty(window, "requestAnimationFrame", {
    configurable: true,
    value: vi.fn((callback: FrameRequestCallback) =>
      window.setTimeout(() => callback(performance.now()), 16),
    ),
    writable: true,
  });
  Object.defineProperty(window, "cancelAnimationFrame", {
    configurable: true,
    value: vi.fn((frame: number) => window.clearTimeout(frame)),
    writable: true,
  });
  Object.defineProperty(Element.prototype, "animate", {
    configurable: true,
    value: vi.fn(() => animationStub()),
    writable: true,
  });
});
