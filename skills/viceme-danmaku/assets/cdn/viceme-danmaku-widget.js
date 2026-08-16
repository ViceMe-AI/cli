;(function () {
  "use strict";

  var VERSION = "1.0.0";
  var DATA_NAME = "ViceMe-Danmaku";
  var ROOT_PREFIX = "viceme-danmaku-root";
  var DEFAULT_CN_ORIGIN = "https://viceme.cn";
  var DEFAULT_GLOBAL_ORIGIN = "https://viceme.ai";
  var DEFAULT_PATH = "/embed/danmaku";
  var bootScript = document.currentScript;

  function ready(callback) {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", callback, { once: true });
    } else {
      callback();
    }
  }

  function findScripts() {
    var current = document.currentScript || bootScript;
    var scripts = [];
    if (current && isDanmakuScript(current)) scripts.push(current);
    Array.prototype.forEach.call(
      document.querySelectorAll(
        'script[data-name="' +
          DATA_NAME +
          '"],script[data-creator-id][data-work-id]',
      ),
      function (script) {
        if (scripts.indexOf(script) === -1) scripts.push(script);
      },
    );
    return scripts;
  }

  function isDanmakuScript(script) {
    return (
      script &&
      script.tagName === "SCRIPT" &&
      (script.getAttribute("data-name") === DATA_NAME ||
        script.hasAttribute("data-creator-id") ||
        script.hasAttribute("data-work-id"))
    );
  }

  function sanitize(value) {
    return String(value || "")
      .trim()
      .replace(/[^a-zA-Z0-9_-]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 80);
  }

  function defaultOrigin(script) {
    var source = script.getAttribute("src") || "";
    try {
      var sourceUrl = new URL(source, window.location.href);
      if (
        sourceUrl.hostname === "localhost" ||
        sourceUrl.hostname === "127.0.0.1" ||
        sourceUrl.hostname === "[::1]"
      ) {
        return sourceUrl.origin;
      }
    } catch {
      // Fall back to the production CDN host heuristic below.
    }
    if (/\.cn(?:\/|$)/i.test(source)) return DEFAULT_CN_ORIGIN;
    return DEFAULT_GLOBAL_ORIGIN;
  }

  function hostURL() {
    return window.location.origin + window.location.pathname;
  }

  function widgetURL(script, mode) {
    var explicit = script.getAttribute("data-widget-url");
    var origin = script.getAttribute("data-widget-origin") || defaultOrigin(script);
    var url = explicit ? new URL(explicit, window.location.href) : new URL(DEFAULT_PATH, origin);
    url.searchParams.set("mode", mode);
    url.searchParams.set("creatorId", script.getAttribute("data-creator-id") || "");
    url.searchParams.set("workId", script.getAttribute("data-work-id") || "");
    url.searchParams.set("theme", script.getAttribute("data-theme") || "auto");
    url.searchParams.set("locale", script.getAttribute("data-locale") || navigator.language || "en");
    url.searchParams.set("host", hostURL());
    url.searchParams.set("sdk", VERSION);
    return url.toString();
  }

  function setFrameBase(frame, title) {
    frame.title = title;
    frame.setAttribute("allowtransparency", "true");
    frame.setAttribute("loading", "eager");
    frame.setAttribute("referrerpolicy", "strict-origin-when-cross-origin");
    frame.style.position = "fixed";
    frame.style.margin = "0";
    frame.style.border = "0";
    frame.style.background = "transparent";
    frame.style.width = "100%";
    frame.style.zIndex = "2147483000";
  }

  function createFrame(root, title, src, cssText) {
    var frame = document.createElement("iframe");
    setFrameBase(frame, title);
    frame.src = src;
    frame.style.cssText += cssText;
    root.appendChild(frame);
    return frame;
  }

  function initScript(script) {
    if (script.getAttribute("data-viceme-mounted") === "true") return null;

    var creatorId = script.getAttribute("data-creator-id");
    var workId = script.getAttribute("data-work-id");
    if (!creatorId || !workId) {
      warn("missing data-creator-id or data-work-id", script);
      return null;
    }

    var rootId =
      ROOT_PREFIX + "-" + sanitize(creatorId) + "-" + sanitize(workId);
    if (document.getElementById(rootId)) {
      script.setAttribute("data-viceme-mounted", "true");
      return document.getElementById(rootId);
    }

    var root = document.createElement("div");
    root.id = rootId;
    root.setAttribute("data-viceme-danmaku", "mounted");
    root.style.position = "fixed";
    root.style.inset = "0";
    root.style.width = "100%";
    root.style.height = "100%";
    root.style.pointerEvents = "none";
    root.style.zIndex = "2147483000";

    var stage = createFrame(
      root,
      "ViceMe Danmaku",
      widgetURL(script, "stage"),
      "inset:0;height:100%;pointer-events:none;",
    );
    stage.setAttribute("aria-hidden", "true");

    var controls = createFrame(
      root,
      "ViceMe Danmaku controls",
      widgetURL(script, "controls"),
      "left:0;right:0;bottom:0;height:calc(88px + env(safe-area-inset-bottom,0px));pointer-events:auto;",
    );

    var modal = createFrame(
      root,
      "ViceMe Danmaku dialog",
      "about:blank",
      "inset:0;height:100%;pointer-events:auto;display:none;",
    );
    modal.setAttribute("data-src", widgetURL(script, "modal"));

    document.body.appendChild(root);
    script.setAttribute("data-viceme-mounted", "true");
    bindMessages(script, controls, modal);
    return root;
  }

  function bindMessages(script, controls, modal) {
    var expectedOrigin = new URL(widgetURL(script, "stage")).origin;
    window.addEventListener("message", function (event) {
      var data = event.data || {};
      if (event.origin !== expectedOrigin || data.source !== "viceme-danmaku") {
        return;
      }
      if (data.action === "resize-controls") {
        var height = Math.max(48, Math.min(360, Number(data.height) || 88));
        controls.style.height =
          "calc(" + height + "px + env(safe-area-inset-bottom,0px))";
      }
      if (data.action === "open-modal") {
        if (modal.src === "about:blank") modal.src = modal.getAttribute("data-src");
        modal.style.display = "block";
      }
      if (data.action === "close-modal") {
        modal.style.display = "none";
      }
    });
  }

  function warn(message, script) {
    if (window.console && typeof window.console.warn === "function") {
      window.console.warn("[ViceMe Danmaku] " + message, script);
    }
  }

  function init() {
    return findScripts().flatMap(function (script) {
      var widget = initScript(script);
      return widget ? [widget] : [];
    });
  }

  window.ViceMeDanmakuWidget = {
    init: init,
    version: VERSION,
  };

  ready(init);
})();
