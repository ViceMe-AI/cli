(function () {
  "use strict";

  var VERSION = "1.0.0";
  var DATA_NAME = "ViceMe-Danmaku";
  var ROOT_PREFIX = "viceme-danmaku-root";
  var DEFAULT_CN_ORIGIN = "https://viceme.cn";
  var DEFAULT_GLOBAL_ORIGIN = "https://viceme.ai";
  var DEFAULT_PATH = "/embed/danmaku";
  var bootScript = document.currentScript;
  var mounts = [];
  var currentAnchor = null;
  var anchorTimer = 0;
  var observersInstalled = false;

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

  function canonicalURL() {
    var canonical = document.querySelector('link[rel="canonical"][href]');
    var href =
      canonical && canonical.href
        ? canonical.href
        : window.location.origin +
          window.location.pathname +
          window.location.search +
          window.location.hash;
    try {
      var url = new URL(href, window.location.href);
      return url.toString();
    } catch {
      return window.location.href;
    }
  }

  function hashString(value) {
    var h1 = 0xdeadbeef ^ value.length;
    var h2 = 0x41c6ce57 ^ value.length;
    for (var i = 0; i < value.length; i += 1) {
      var ch = value.charCodeAt(i);
      h1 = Math.imul(h1 ^ ch, 2654435761);
      h2 = Math.imul(h2 ^ ch, 1597334677);
    }
    h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507);
    h1 ^= Math.imul(h2 ^ (h2 >>> 13), 3266489909);
    h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507);
    h2 ^= Math.imul(h1 ^ (h1 >>> 13), 3266489909);
    return (
      (h2 >>> 0).toString(36).padStart(7, "0") +
      (h1 >>> 0).toString(36).padStart(7, "0")
    ).slice(0, 16);
  }

  function scrollBucket() {
    var root = document.documentElement;
    var body = document.body;
    var scrollTop =
      window.pageYOffset || root.scrollTop || (body && body.scrollTop) || 0;
    var scrollHeight = Math.max(
      root.scrollHeight,
      root.offsetHeight,
      root.clientHeight,
      body ? body.scrollHeight : 0,
      body ? body.offsetHeight : 0,
    );
    var maxScroll = Math.max(0, scrollHeight - window.innerHeight);
    if (maxScroll <= 1) return "0-100";
    var percent = Math.max(0, Math.min(99, (scrollTop / maxScroll) * 100));
    var start = Math.min(90, Math.floor(percent / 10) * 10);
    return start + "-" + (start + 10);
  }

  function readAutoAnchor() {
    var url = canonicalURL();
    return {
      anchorKey: "page:" + hashString(url) + ":scroll:" + scrollBucket(),
    };
  }

  function updateAnchor(options) {
    var next = readAutoAnchor();
    if (currentAnchor && currentAnchor.anchorKey === next.anchorKey) {
      return currentAnchor;
    }
    currentAnchor = next;
    if (!options || options.notify !== false) {
      mounts.forEach(postAnchor);
    }
    return currentAnchor;
  }

  function scheduleAnchorUpdate() {
    window.clearTimeout(anchorTimer);
    anchorTimer = window.setTimeout(updateAnchor, 120);
  }

  function installAnchorObservers() {
    if (observersInstalled) return;
    observersInstalled = true;
    window.addEventListener("scroll", scheduleAnchorUpdate, { passive: true });
    window.addEventListener("resize", scheduleAnchorUpdate);
    window.addEventListener("popstate", scheduleAnchorUpdate);
    window.addEventListener("hashchange", scheduleAnchorUpdate);

    ["pushState", "replaceState"].forEach(function (name) {
      var original = history[name];
      if (typeof original !== "function") return;
      history[name] = function () {
        var result = original.apply(this, arguments);
        scheduleAnchorUpdate();
        return result;
      };
    });
  }

  function widgetURL(script, mode) {
    var explicit = script.getAttribute("data-widget-url");
    var origin =
      script.getAttribute("data-widget-origin") || defaultOrigin(script);
    var url = explicit
      ? new URL(explicit, window.location.href)
      : new URL(DEFAULT_PATH, origin);
    url.searchParams.set("mode", mode);
    url.searchParams.set(
      "creatorId",
      script.getAttribute("data-creator-id") || "",
    );
    url.searchParams.set("workId", script.getAttribute("data-work-id") || "");
    url.searchParams.set("theme", script.getAttribute("data-theme") || "auto");
    url.searchParams.set(
      "locale",
      script.getAttribute("data-locale") || navigator.language || "en",
    );
    url.searchParams.set("host", canonicalURL());
    url.searchParams.set("sdk", VERSION);
    if (currentAnchor) {
      url.searchParams.set("anchorKey", currentAnchor.anchorKey);
    }
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
      "left:0;right:0;bottom:0;height:calc(136px + env(safe-area-inset-bottom,0px));pointer-events:auto;",
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
    var mount = {
      root: root,
      script: script,
      stage: stage,
      controls: controls,
      modal: modal,
      expectedOrigin: new URL(widgetURL(script, "stage")).origin,
    };
    mounts.push(mount);
    stage.addEventListener("load", function () {
      postAnchor(mount);
    });
    controls.addEventListener("load", function () {
      postAnchor(mount);
    });
    bindMessages(script, controls, modal, mount);
    postAnchor(mount);
    return root;
  }

  function bindMessages(script, controls, modal, mount) {
    var expectedOrigin = new URL(widgetURL(script, "stage")).origin;
    window.addEventListener("message", function (event) {
      var data = event.data || {};
      if (event.origin !== expectedOrigin || data.source !== "viceme-danmaku") {
        return;
      }
      if (data.action === "resize-controls") {
        var height = Math.max(136, Math.min(360, Number(data.height) || 136));
        controls.style.height =
          "calc(" + height + "px + env(safe-area-inset-bottom,0px))";
      }
      if (data.action === "request-anchor") {
        postAnchor(mount);
      }
      if (data.action === "open-modal") {
        if (modal.src === "about:blank")
          modal.src = modal.getAttribute("data-src");
        modal.style.display = "block";
      }
      if (data.action === "close-modal") {
        modal.style.display = "none";
      }
    });
  }

  function postAnchor(mount) {
    var message = currentAnchor
      ? {
          source: "viceme-danmaku",
          action: "anchor-change",
          anchorKey: currentAnchor.anchorKey,
        }
      : {
          source: "viceme-danmaku",
          action: "anchor-change",
          anchorKey: null,
        };
    [mount.stage, mount.controls].forEach(function (frame) {
      if (frame.contentWindow) {
        frame.contentWindow.postMessage(message, "*");
      }
    });
  }

  function warn(message, script) {
    if (window.console && typeof window.console.warn === "function") {
      window.console.warn("[ViceMe Danmaku] " + message, script);
    }
  }

  function init() {
    updateAnchor({ notify: false });
    installAnchorObservers();
    return findScripts().flatMap(function (script) {
      var widget = initScript(script);
      return widget ? [widget] : [];
    });
  }

  window.ViceMeDanmakuWidget = {
    init: init,
    refresh: updateAnchor,
    version: VERSION,
  };

  ready(init);
})();
