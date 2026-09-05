const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = name => readFileSync(path.join(__dirname, "../widgets", name + ".html"), "utf8");
class Element {
  constructor(tag = "div") { this.tag = tag; this.children = []; this.events = {}; this.dataset = {}; this.hidden = false; this.textContent = ""; this.disabled = false; }
  append(...children) { this.children.push(...children); }
  setAttribute(key, value) { this[key] = value; }
  addEventListener(name, action) { this.events[name] = action; }
  querySelectorAll(tag) { return this.children.flatMap(child => [...(child.tag === tag ? [child] : []), ...child.querySelectorAll(tag)]); }
}
function mount(name, data, { now = Date.UTC(2026, 8, 5), svg = true, sendPrompt } = {}) {
  const fields = new Map();
  const root = new Element(); root.isConnected = true;
  root.classList = { contains: value => value === "viceme-" + name };
  root.querySelector = selector => {
    if (!fields.has(selector)) fields.set(selector, new Element());
    return fields.get(selector);
  };
  const field = name => root.querySelector('[data-field="' + name + '"]');
  field("qr").querySelector = () => svg ? new Element("svg") : null;
  const listeners = new Map(); const intervals = new Map();
  const document = {
    currentScript: { previousElementSibling: root }, createElement: tag => new Element(tag),
    addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name),
  };
  const window = { addEventListener: document.addEventListener, removeEventListener: document.removeEventListener };
  class Clock extends Date { static now() { return now; } }
  const code = source(name).match(/<script>([\s\S]*?)<\/script>\s*$/)[1].replace("__WIDGET_DATA__", JSON.stringify(data));
  vm.runInNewContext(code, {
    document, window, Date: Clock, Intl, sendPrompt,
    setInterval: fn => { intervals.set(1, fn); return 1; }, clearInterval: id => intervals.delete(id),
  });
  return { root, fields, field, intervals, listeners,
    advance: value => { now = value; for (const fn of [...intervals.values()]) fn(); },
    event: name => listeners.get(name)?.(),
  };
}
const start = Date.UTC(2026, 8, 5);
const order = { title: "通用订单", amountCents: 1990, currency: "CNY", paymentMethodLabel: "微信支付", status: "PENDING", expiresAt: new Date(start + 60000).toISOString(), locale: "zh-CN" };

test("payment uses absolute expiry across ticks, reload and clock rollback", () => {
  const view = mount("payment", order);
  assert.equal(view.field("countdown").textContent, "01:00");
  assert.equal(view.field("qr").hidden, false);
  view.advance(start + 45000);
  assert.equal(view.field("countdown").textContent, "00:15");
  const reloaded = mount("payment", order, { now: start + 45000 });
  assert.equal(reloaded.field("countdown").textContent, "00:15");
  view.advance(start + 61000);
  assert.equal(view.root.dataset.phase, "expired");
  assert.equal(view.field("qr").hidden, true);
  assert.equal(view.field("state").textContent, "二维码已过期");
  assert.equal(view.intervals.size, 0);
  assert.equal(view.listeners.size, 0);
  view.advance(start);
  assert.equal(view.field("qr").hidden, true);
});
test("invalid dates fail closed, missing QR does not masquerade as an active payment", () => {
  for (const expiry of ["", "invalid"]) {
    const view = mount("payment", { ...order, expiresAt: expiry });
    assert.equal(view.root.dataset.phase, "unavailable");
    assert.equal(view.field("qr").hidden, true);
    assert.equal(view.intervals.size, 0);
  }
  assert.equal(mount("payment", order, { svg: false }).root.dataset.phase, "unavailable");
  assert.equal(mount("payment", order, { svg: false, now: start + 61000 }).root.dataset.phase, "expired");
});
test("only server status can show paid, and detached Widgets release listeners", () => {
  const paid = mount("payment", { ...order, status: "PAID" });
  assert.equal(paid.field("state").textContent, "支付成功");
  assert.equal(paid.field("qr").hidden, true);
  assert.equal(paid.intervals.size, 0);
  const pending = mount("payment", order);
  pending.root.isConnected = false; pending.advance(start + 1000);
  assert.equal(pending.intervals.size, 0); assert.equal(pending.listeners.size, 0);
});
test("payment template contains no business flow, clipboard, network or query button", () => {
  const html = source("payment");
  assert.doesNotMatch(html, /<button|<img|<script[^>]+src|fetch\(|XMLHttpRequest|navigator\.clipboard|sendPrompt|canghe|安装|试用|查询支付|已付款，但/);
  assert.equal((html.match(/<script>/g) || []).length, 1);
  assert.ok(html.indexOf("<script>") > html.indexOf("</section>"));
});
test("onboarding sends the exact prompt once and never treats titles as HTML", async () => {
  const prompt = "使用任意 Skill。\n<script>not code</script>";
  const data = { skillName: "任意 Skill", examples: [{ title: "示例一", prompt }, { title: "示例二", prompt: "another" }] };
  const calls = [];
  const view = mount("onboarding", data, { sendPrompt: async value => { calls.push(value); } });
  const buttons = view.field("examples").querySelectorAll("button");
  const sent = buttons[0].events.click();
  await buttons[1].events.click(); await sent;
  assert.deepEqual(calls, [prompt]);
  assert.ok(buttons.every(button => button.disabled));
  assert.equal(view.root.querySelector('[role="status"]').textContent, "已发送到对话。");
  assert.equal(view.field("examples").querySelectorAll("p")[0].textContent, prompt);
  assert.doesNotMatch(source("onboarding"), /innerHTML|navigator\.clipboard|fetch\(/);
});
test("onboarding falls back to readable prompts and avoids automatic resend on uncertainty", async () => {
  const data = { skillName: "Example", locale: "en-US", examples: [{ title: "Try", prompt: "exact" }] };
  const fallback = mount("onboarding", data);
  assert.equal(fallback.field("examples").querySelectorAll("button")[0].disabled, true);
  assert.equal(fallback.field("examples").querySelectorAll("p")[0].textContent, "exact");
  const uncertain = mount("onboarding", data, { sendPrompt: async () => { throw Error("unknown delivery"); } });
  const button = uncertain.field("examples").querySelectorAll("button")[0];
  await button.events.click();
  assert.equal(button.disabled, true);
  assert.match(uncertain.root.querySelector('[role="status"]').textContent, /Check the conversation/);
});
