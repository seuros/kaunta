/* Safe DOM rendering for array-valued Datastar signals. No untrusted HTML. */
(() => {
  "use strict";
  const byId = (id) => document.getElementById(id);
  const node = (tag, text) => {
    const result = document.createElement(tag);
    if (text !== undefined) result.textContent = String(text);
    return result;
  };
  const headers = () => ({
    "X-CSRF-Token": decodeURIComponent(document.cookie.split("; ")
      .find((cookie) => cookie.startsWith("kaunta_csrf="))?.slice(12) || ""),
  });
  const query = (website, days, filters) => {
    const params = new URLSearchParams({website_id: website, days});
    for (const [key, value] of Object.entries(filters || {})) {
      if (value) params.set(key, value);
    }
    return params.toString();
  };
  const REARM = "$statsLoading = true; $chartLoading = true; $breakdownLoading = true; $mapLoading = true";
  function emptyState(message, hint, loading = false) {
    const state = node("div");
    state.className = `empty-state-mini${loading ? " is-loading" : ""}`;
    state.setAttribute("role", "status");
    state.append(node("p", message));
    if (hint) state.append(node("small", hint));
    return state;
  }
  function table(id, items, keys, loading = false, error = false, filter = null) {
    const body = byId(id);
    if (!body) return;
    const rows = items.map((item) => {
      const row = node("tr");
      keys.forEach((key, index) => {
        if (index === 0 && filter && item[filter.key]) {
          const cell = node("td");
          const button = node("button", item[key] ?? "");
          button.type = "button";
          button.className = "filter-link";
          button.setAttribute(
            "data-on:click",
            `${filter.signal} = ${JSON.stringify(String(item[filter.key]))}; ${REARM}`,
          );
          cell.append(button);
          row.append(cell);
        } else {
          row.append(node("td", item[key] ?? ""));
        }
      });
      return row;
    });
    if (!rows.length || loading || error) {
      rows.length = 0;
      const row = node("tr");
      const cell = node("td");
      cell.append(emptyState(loading ? "Loading visits…" : error ? "Visits could not be loaded." : "No visits in this period.",
        loading || error ? "" : "Visits will appear once your tracking code is installed.", loading));
      cell.colSpan = keys.length;
      row.append(cell);
      rows.push(row);
    }
    body.replaceChildren(...rows);
  }
  function websites(items, selected) {
    const select = byId("website-select");
    const signature = JSON.stringify(items);
    if (select.dataset.items !== signature) {
      select.dataset.items = signature;
      select.replaceChildren(...items.map((item) => new Option(item.name || item.domain, item.id)));
    }
    select.value = selected;
  }
  function chart(items, loading = false, error = false) {
    const target = byId("pageviews-chart");
    target.setAttribute("aria-busy", String(loading));
    if (loading || error || !items.some((item) => Number(item.value) > 0)) {
      target.replaceChildren(emptyState(loading ? "Loading pageviews…" : error ? "Pageviews could not be loaded." : "No pageviews in this period.",
        loading || error ? "" : "Your first visit will bring this chart to life.", loading));
      return;
    }
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 800 200");
    svg.setAttribute("role", "img");
    svg.setAttribute("aria-label", "Pageviews over time; values in the chart data table");
    const line = document.createElementNS(svg.namespaceURI, "polyline");
    const maximum = Math.max(1, ...items.map((item) => item.value));
    line.setAttribute("points", items.map((item, i) =>
      `${10 + i * 780 / Math.max(1, items.length - 1)},${190 - item.value / maximum * 180}`).join(" "));
    line.setAttribute("fill", "none");
    line.setAttribute("stroke", "#3b82f6");
    line.setAttribute("stroke-width", "3");
    svg.append(line);
    target.replaceChildren(svg);
  }
  async function jsonRequest(url, method, data) {
    const response = await fetch(url, {
      method, headers: {...headers(), "Content-Type": "application/json"},
      body: JSON.stringify(data),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(error.error || `Request failed (${response.status})`);
    }
    return response.json();
  }
  function cards(items) {
    const cards = items.map((item) => {
      const card = node("article");
      card.append(node("h2", item.name), node("p", item.domain), node("p", `ID: ${item.id}`));
      if (item.deletion_in_days !== undefined) {
        const notice = node("p");
        notice.className = "pending-delete";
        notice.setAttribute("role", "alert");
        notice.append(node("span", item.deletion_in_days === 0
          ? "Deletion pending: removal is due."
          : `Deletion pending: data is removed in ${item.deletion_in_days} day${item.deletion_in_days === 1 ? "" : "s"}.`));
        const keep = node("button", "Keep this website");
        keep.type = "button";
        keep.addEventListener("click", async () => {
          keep.disabled = true;
          try {
            await jsonRequest(`/api/websites/${item.id}/restore`, "POST", {});
            notice.replaceChildren(node("span", "Deletion cancelled."));
          } catch (error) {
            notice.append(node("span", ` ${error.message}`));
            keep.disabled = false;
          }
        });
        notice.append(keep);
        card.append(notice);
      }
      const form = node("form");
      const field = (label, value) => {
        const wrapper = node("label", label);
        const input = node("input");
        input.value = value;
        wrapper.append(input);
        form.append(wrapper);
        return input;
      };
      const name = field("Name", item.name);
      const domains = field("Allowed domains (comma separated)", item.allowed_domains.join(", "));
      const savedDomains = new Set(item.allowed_domains);
      const publicLabel = node("label", "Public stats ");
      const publicStats = node("input");
      publicStats.type = "checkbox";
      publicStats.checked = item.public_stats_enabled;
      publicLabel.append(publicStats);
      form.append(publicLabel);
      const save = node("button", "Save website");
      save.type = "submit";
      const status = node("p");
      status.setAttribute("role", "status");
      form.append(save, status);
      form.addEventListener("submit", async (event) => {
        event.preventDefault();
        save.disabled = true;
        try {
          await jsonRequest(`/api/websites/${item.id}`, "PUT", {
            name: name.value,
          });
          const wanted = new Set(domains.value.split(",").map((s) => s.trim()).filter(Boolean));
          for (const domain of wanted) {
            if (!savedDomains.has(domain)) {
              await jsonRequest(`/api/websites/${item.id}/domains`, "POST", {domain});
              savedDomains.add(domain);
            }
          }
          for (const domain of savedDomains) {
            if (!wanted.has(domain)) {
              await jsonRequest(`/api/websites/${item.id}/domains`, "DELETE", {domain});
              savedDomains.delete(domain);
            }
          }
          await jsonRequest(`/api/websites/${item.id}/public-stats`, "PATCH", {enabled: publicStats.checked});
          status.textContent = "Saved";
        } catch (error) { status.textContent = error.message; }
        finally { save.disabled = false; }
      });
      const code = node("pre", `<script defer src="${location.origin}/kaunta.js" data-website-id="${item.id}"></script>`);
      card.append(form, node("h3", "Tracking code"), code);
      return card;
    });
    byId("websites-container").replaceChildren(...cards);
  }
  const goalForms = new Map();
  function goals(items) {
    goalForms.clear();
    const cards = items.map((item) => {
      const card = node("article");
      card.className = "goal-card";
      card.append(node("h2", item.name), node("p", item.target_url || item.target_event));
      const kind = node("span", item.target_url ? "Page view" : "Custom event");
      kind.className = "goal-kind";
      card.prepend(kind);
      const edit = node("button", "Edit");
      edit.type = "button";
      goalForms.set(item.id, {
        name: item.name, type: item.target_url ? "page_view" : "custom_event",
        value: item.target_url || item.target_event,
      });
      edit.setAttribute("data-on:click", `$goalId = '${item.id}'; $goalForm = window.kaunta.goalForm('${item.id}')`);
      const remove = node("button", "Delete");
      remove.className = "button-danger";
      remove.type = "button";
      remove.setAttribute("data-on:click", `if (confirm('Delete this goal?')) @delete('/api/dashboard/goals/${item.id}', {headers: window.kaunta.headers()})`);
      const analytics = node("button", "Analytics");
      analytics.className = "button-accent";
      analytics.type = "button";
      const result = node("p");
      result.setAttribute("role", "status");
      result.className = "goal-result";
      analytics.addEventListener("click", async () => {
        analytics.disabled = true;
        result.textContent = "Loading conversions…";
        try {
          const response = await fetch(`/api/dashboard/goals/${item.id}/analytics?days=${encodeURIComponent(byId("days").value)}`);
          if (!response.ok) throw new Error(`Request failed (${response.status})`);
          const stats = await response.json();
          result.replaceChildren(...[
            [stats.completions, "Completions"], [stats.unique_sessions, "Sessions"],
            [`${stats.conversion_rate.toFixed(1)}%`, "Conversion"],
          ].map(([value, label]) => {
            const stat = node("span");
            stat.append(node("strong", value), node("small", label));
            return stat;
          }));
        } catch (error) { result.textContent = error.message; }
        finally { analytics.disabled = false; }
      });
      const actions = node("div");
      actions.className = "goal-actions";
      actions.append(analytics, edit, remove);
      card.append(actions, result);
      return card;
    });
    byId("goals-container").replaceChildren(...cards);
  }
  const delta = (current, previous) => {
    if (!previous) return current ? "(new)" : "";
    const pct = Math.round(((current - previous) / previous) * 100);
    return `(${pct >= 0 ? "+" : ""}${pct}%)`;
  };
  window.kaunta = Object.assign(window.kaunta || {}, {headers, query, table, websites, chart, cards, goals, delta, emptyState,
    goalForm: (id) => ({...goalForms.get(id)}),
    breakdown: (items, loading, error, dimension) => {
      const filterSignals = {
        page: "$filterPage", country: "$filterCountry",
        browser: "$filterBrowser", device: "$filterDevice",
      };
      const signal = filterSignals[dimension];
      return table("breakdown-data", items.map((item) => ({
        name: item.path || item.name, count: item.views || item.count,
      })), ["name", "count"], loading, error, signal ? {signal, key: "name"} : null);
    },
  });
  async function renderExclusions() {
    const container = byId("exclusions");
    if (!container) return;
    const row = (label, button) => {
      const line = node("div");
      line.className = "exclusion-row";
      line.append(node("span", label));
      if (button) line.append(button);
      return line;
    };
    try {
      const [me, list] = await Promise.all([
        fetch("/api/dashboard/whoami").then((r) => r.json()),
        fetch("/api/dashboard/exclusions").then((r) => r.json()),
      ]);
      const rows = [];
      const self = node("div");
      self.className = "exclusion-self";
      self.append(node("span", `Your address right now: ${me.ip}`));
      if (me.excluded) {
        self.append(node("strong", "already excluded"));
      } else {
        const add = node("button", "Exclude it");
        add.type = "button";
        add.addEventListener("click", async () => {
          add.disabled = true;
          try {
            await jsonRequest("/api/dashboard/exclusions", "POST", {rule: me.ip, note: "added from the dashboard"});
            await renderExclusions();
          } catch (error) { self.append(node("span", ` ${error.message}`)); add.disabled = false; }
        });
        self.append(add);
      }
      rows.push(self);
      for (const item of list.exclusions ?? []) {
        const remove = node("button", "Remove");
        remove.type = "button";
        remove.className = "button-danger";
        remove.addEventListener("click", async () => {
          remove.disabled = true;
          try {
            await jsonRequest(`/api/dashboard/exclusions/${encodeURIComponent(item.excluded_address_id)}`, "DELETE");
            await renderExclusions();
          } catch (error) { remove.disabled = false; }
        });
        rows.push(row(item.note ? `${item.rule} · ${item.note}` : item.rule, remove));
      }
      if (!(list.exclusions ?? []).length) rows.push(row("No addresses excluded yet."));
      container.replaceChildren(...rows);
    } catch (error) {
      container.replaceChildren(node("p", `Exclusions could not be loaded: ${error.message}`));
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    initLiveFeed();
    renderExclusions();
    byId("login-form")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = event.currentTarget;
      const button = form.querySelector("button");
      button.disabled = true;
      byId("login-error").textContent = "";
      try {
        await jsonRequest("/api/auth/login", "POST", Object.fromEntries(new FormData(form)));
        location.assign("/dashboard");
      } catch (error) { byId("login-error").textContent = error.message; }
      finally { button.disabled = false; }
    });
  });
  function initLiveFeed() {
    const feed = byId("live-feed");
    if (!feed) return;
    let delay = 3000;
    const row = (event) => {
      const line = node("div");
      line.className = "live-row";
      const time = new Date(event.created_at);
      line.append(
        node("time", Number.isNaN(time.getTime()) ? "" : time.toTimeString().slice(0, 8)),
        node("span", event.path || event.title || event.type || "event"),
      );
      return line;
    };
    const connect = () => {
      const socket = new WebSocket(
        `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/realtime`,
      );
      socket.addEventListener("open", () => { delay = 3000; });
      socket.addEventListener("message", (message) => {
        let event;
        try { event = JSON.parse(message.data); } catch { return; }
        const selected = byId("website-select")?.value;
        if (!event.website_id || (selected && event.website_id !== selected)) return;
        if (feed.classList.contains("empty-state-mini")) {
          feed.classList.remove("empty-state-mini");
          feed.replaceChildren();
        }
        feed.prepend(row(event));
        while (feed.childElementCount > 10) feed.lastElementChild.remove();
      });
      socket.addEventListener("close", () => {
        setTimeout(connect, delay);
        delay = Math.min(delay * 2, 30000);
      });
    };
    connect();
  }

  document.addEventListener("datastar-fetch", (event) => {
    const target = byId("request-error");
    if (target && event.detail.type === "error") {
      target.textContent = "Request failed. Check your connection or sign in again.";
    }
  });
})();
