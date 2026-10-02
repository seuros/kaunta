const TIMEOUT_MS = Number(window.KAUNTA_TIMEOUT_MS ?? 30000);
const pending = new Map();
let nextId = 1;
let onResult = () => {};
let onTeardown = () => {};

const send = (message) => window.parent.postMessage({ jsonrpc: "2.0", ...message }, "*");

const request = (method, params) =>
  new Promise((resolve, reject) => {
    const id = nextId++;
    const expiry = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`${method} timed out`));
    }, TIMEOUT_MS);
    pending.set(id, { resolve, reject, expiry });
    send({ id, method, params });
  });

const call = (name, args) => request("tools/call", { name, arguments: args });
const el = (id) => document.getElementById(id);
const onToolResult = (fn) => { onResult = fn; };
const onResourceTeardown = (fn) => { onTeardown = fn; };

window.addEventListener("message", ({ source, data }) => {
  if (source !== window.parent || data?.jsonrpc !== "2.0") return;
  if (data.method === undefined) {
    const waiting = pending.get(data.id);
    if (!waiting) return;
    pending.delete(data.id);
    clearTimeout(waiting.expiry);
    data.error ? waiting.reject(new Error(data.error.message)) : waiting.resolve(data.result);
  } else if (data.id !== undefined) {
    if (data.method === "ui/resource-teardown") {
      onTeardown();
      send({ id: data.id, result: {} });
    } else if (data.method === "ping") {
      send({ id: data.id, result: {} });
    } else {
      send({ id: data.id, error: { code: -32601, message: `Method not found: ${data.method}` } });
    }
  } else if (data.method === "ui/notifications/tool-result") {
    onResult(data.params);
  }
});

const payloadOf = (result) => result?.structuredContent ?? result;

function renderPeriods(panel, refreshTool, onPanel) {
  const group = el("periods");
  if (!group) return;
  group.replaceChildren(...[1, 7, 30, 90].map((days) => {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = days === 1 ? "Today" : `${days}d`;
    button.setAttribute("aria-pressed", String(days === panel.period_days));
    button.addEventListener("click", async () => {
      const buttons = [...group.querySelectorAll("button")];
      buttons.forEach((b) => (b.disabled = true));
      try {
        onPanel(payloadOf(await call(refreshTool, { website: panel.website, days })));
      } finally {
        buttons.forEach((b) => (b.disabled = false));
      }
    });
    return button;
  }));
}
