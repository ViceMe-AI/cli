// Local-only onboarding fixture. Buttons echo prompts without sending them.
const { createServer } = require("node:http");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const widgets = join(__dirname, "../widgets");
const encode = data => JSON.stringify(data).replace(/[<>&\u2028\u2029]/g, char => "\\u" + char.charCodeAt(0).toString(16).padStart(4, "0"));
const server = createServer((req, res) => {
  const url = new URL(req.url, "http://localhost");
  const onboarding = { skillName: "任意 Skill", locale: "zh-CN", examples: [{ title: "从一个小任务开始", prompt: "请使用这个 Skill，根据它支持的能力完成一个最小示例，并说明输入和结果。" }, { title: "带着自己的材料来", prompt: "请使用这个 Skill 处理我接下来提供的材料。先确认需要哪些输入，再开始任务。" }, { title: "把已有结果再做好一点", prompt: "请使用这个 Skill，根据我给出的目标改进已有结果，并说明本次修改的要点。" }] };
  const bridge = '<script>window.sendPrompt=async prompt=>{document.getElementById("prompt-result").textContent=prompt;};</script>';
  const fill = (name, data) => readFileSync(join(widgets, name + ".html"), "utf8").replace("__WIDGET_DATA__", () => encode(data));
  res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" });
  res.end(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ViceMe 通用 Widget 验收</title><style>body{margin:0;padding:24px;font:14px/1.6 system-ui;background:#fafbf9;color:#26342b}main{max-width:640px;margin:auto}.fixture{background:white;border:1px solid #e1e7e2;border-radius:12px;padding:24px;margin:20px 0}nav{display:flex;gap:18px;flex-wrap:wrap}a{color:#28674b}pre{white-space:pre-wrap;overflow-wrap:anywhere}iframe{width:100%;min-height:640px;border:1px solid #e1e7e2;border-radius:12px;background:#f2f2f2}@media(max-width:480px){body{padding:12px}.fixture{padding:16px}}</style></head><body><main><h1>通用 Widget</h1><p>本地上手示例验收：按钮仅回显口令。付款页面由 Shop 官方收银台测试覆盖。</p><div class="fixture">${bridge}${fill("onboarding", onboarding)}</div><pre id="prompt-result" role="status">尚未点击示例</pre></main><script>window.sendPrompt=async prompt=>{document.getElementById("prompt-result").textContent=prompt;};</script></body></html>`);
});
server.listen(0, "127.0.0.1", () => console.log(`http://127.0.0.1:${server.address().port}/`));
