/**
 * Kaunta Campaigns Dashboard - Datastar Edition
 * Minimal JS for initialization and sort helpers
 * Data flows via SSE signals from the server
 */

// Sort handler - called from SSE-rendered HTML onclick
window.campaignsSortBy = function (dimension, column) {
  // Get current sort state from Datastar signals
  const container = document.getElementById("campaigns-container");
  if (!container || !container._dsSignals) {
    console.error("Cannot access Datastar signals");
    return;
  }

  const signals = container._dsSignals;
  const sort = signals.sort || {};
  const currentSort = sort[dimension] || { column: "count", direction: "desc" };

  // Toggle direction if same column, otherwise default to desc
  let newDirection = "desc";
  if (currentSort.column === column) {
    newDirection = currentSort.direction === "asc" ? "desc" : "asc";
  }

  // Update signals
  const newSort = { ...sort };
  newSort[dimension] = { column: column, direction: newDirection };

  // Dispatch custom event for Datastar to pick up
  container.dispatchEvent(
    new CustomEvent("campaigns:sort", {
      detail: { dimension, column, direction: newDirection },
    }),
  );

  // Trigger SSE fetch with new sort params
  const website = signals.selectedWebsite;
  if (website) {
    const url = `/api/dashboard/campaigns-ds?website=${website}&dimension=${dimension}&sort_by=${column}&sort_order=${newDirection}`;
    // Use Datastar's fetch mechanism
    window.dispatchEvent(
      new CustomEvent("datastar:fetch", {
        detail: { url, method: "GET" },
      }),
    );
  }
};

// Generate table HTML for a UTM dimension (called from Go handler via ExecuteScript)
window.renderUTMTable = function (dimension, data, sortColumn, sortDirection) {
  const container = document.getElementById(`utm-${dimension}-table`);
  if (!container) {
    console.error(`Container utm-${dimension}-table not found`);
    return;
  }

  if (!data || data.length === 0) {
    container.innerHTML = `
      <div class="empty-state-mini">
        <div>[=]</div>
        <div>No UTM ${dimension} data yet</div>
      </div>
    `;
    return;
  }

  const nameLabel = dimension.charAt(0).toUpperCase() + dimension.slice(1);
  const nameArrow =
    sortColumn === "name" ? (sortDirection === "asc" ? " [^]" : " [v]") : "";
  const countArrow =
    sortColumn === "count" ? (sortDirection === "asc" ? " [^]" : " [v]") : "";

  let rows = "";
  for (const item of data) {
    const count =
      typeof item.count === "number" ? item.count.toLocaleString() : item.count;
    rows += `<tr>
      <td>${escapeHtml(item.name)}</td>
      <td style="text-align: right; font-weight: 500; color: var(--accent-color)">${count}</td>
    </tr>`;
  }

  container.innerHTML = `
    <table class="glass card">
      <thead>
        <tr>
          <th
            onclick="campaignsSortBy('${dimension}', 'name')"
            style="cursor: pointer; user-select: none"
            class="sortable-header"
          >
            <span>${nameLabel}</span>
            <span style="opacity: 0.7">${nameArrow}</span>
          </th>
          <th
            onclick="campaignsSortBy('${dimension}', 'count')"
            style="text-align: right; cursor: pointer; user-select: none"
            class="sortable-header"
          >
            <span>Count</span>
            <span style="opacity: 0.7">${countArrow}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        ${rows}
      </tbody>
    </table>
  `;
};

// Helper to escape HTML
function escapeHtml(text) {
  if (text === null || text === undefined) return "";
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// Get CSRF token from cookie
window.getCsrfToken = function () {
  const value = "; " + document.cookie;
  const parts = value.split("; kaunta_csrf=");
  if (parts.length === 2) return parts.pop().split(";").shift();
  return "";
};
