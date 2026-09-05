// Local-only visual fixture. The original fragments are served unchanged apart
// from their documented data slots. No order, payment or real prompt is sent.
const { createServer } = require("node:http");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");
const widgets = join(__dirname, "../widgets");
const matrix = spawnSync("python3", ["-c", "import json,sys;sys.path.insert(0,sys.argv[1]);from qrcodegen import QrCode;q=QrCode.encode_text('VICEME_WIDGET_PREVIEW_ONLY',QrCode.Ecc.MEDIUM);print(json.dumps([[q.get_module(x,y) for x in range(q.get_size())] for y in range(q.get_size())]))", widgets], { encoding: "utf8", env: { ...process.env, PYTHONDONTWRITEBYTECODE: "1" } });
if (matrix.status !== 0) throw Error(matrix.stderr);
const rows = JSON.parse(matrix.stdout);
const path = rows.flatMap((row, y) => row.flatMap((dark, x) => dark ? [`M${x + 4},${y + 4}h1v1h-1z`] : [])).join("");
const svg = `<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="仅用于测试的二维码" viewBox="0 0 ${rows.length + 8} ${rows.length + 8}" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="white"/><path fill="black" d="${path}"/></svg>`;
const encode = data => JSON.stringify(data).replace(/[<>&\u2028\u2029]/g, char => "\\u" + char.charCodeAt(0).toString(16).padStart(4, "0"));
const startedAt = Date.now();
const server = createServer((req, res) => {
  const url = new URL(req.url, "http://localhost");
  const state = url.searchParams.get("state");
  const order = { title: "通用订单 · 演示", amountCents: 1990, currency: "CNY", paymentMethodLabel: "微信支付", status: state === "paid" ? "PAID" : "PENDING", expiresAt: new Date(startedAt + (state === "expired" ? -1 : 3600000)).toISOString(), locale: "zh-CN" };
  const onboarding = { skillName: "任意 Skill", locale: "zh-CN", examples: [{ title: "从一个小任务开始", prompt: "请使用这个 Skill，根据它支持的能力完成一个最小示例，并说明输入和结果。" }, { title: "带着自己的材料来", prompt: "请使用这个 Skill 处理我接下来提供的材料。先确认需要哪些输入，再开始任务。" }, { title: "把已有结果再做好一点", prompt: "请使用这个 Skill，根据我给出的目标改进已有结果，并说明本次修改的要点。" }] };
  const bridge = '<script>window.sendPrompt=async prompt=>{document.getElementById("prompt-result").textContent=prompt;};</script>';
  const fragment = (name, data) => bridge + readFileSync(join(widgets, name + ".html"), "utf8").replace("__WIDGET_DATA__", () => encode(data)).replace("__QR_SVG__", () => svg);
  res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" });
  res.end(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ViceMe 通用 Widget 验收</title><style>body{margin:0;padding:24px;font:14px/1.6 system-ui;background:#fafbf9;color:#26342b}main{max-width:640px;margin:auto}.fixture{background:white;border:1px solid #e1e7e2;border-radius:12px;padding:24px;margin:20px 0}nav{display:flex;gap:18px;flex-wrap:wrap}a{color:#28674b}pre{white-space:pre-wrap;overflow-wrap:anywhere}@media(max-width:480px){body{padding:12px}.fixture{padding:16px}}</style></head><body><main><h1>通用 Widget</h1><p>本地验收：测试二维码不用于付款，按钮仅回显口令。</p><nav><a href="/">等待支付</a><a href="/?state=expired">二维码过期</a><a href="/?state=paid">付款成功</a></nav><div class="fixture">${fragment("onboarding", onboarding)}</div><pre id="prompt-result" role="status">尚未点击示例</pre><div class="fixture">${fragment("payment", order)}</div></main><script>window.sendPrompt=async prompt=>{document.getElementById("prompt-result").textContent=prompt;};</script></body></html>`);
});
server.listen(0, "127.0.0.1", () => console.log(`http://127.0.0.1:${server.address().port}/`));
