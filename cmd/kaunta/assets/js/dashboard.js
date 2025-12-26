/**
 * Kaunta Dashboard - Datastar Edition
 * Minimal JS for Chart.js initialization and helper functions
 * Data flows via SSE signals from the server
 */

// Chart instance reference
let pageviewsChart = null;

// Icon helpers (called from SSE-rendered HTML)
window.countryToFlag = function(code) {
  if (!code || code.length !== 2) return "";
  const codePoints = code
    .toUpperCase()
    .split("")
    .map((char) => 127397 + char.charCodeAt(0));
  return String.fromCodePoint(...codePoints);
};

window.browserIcon = function(name) {
  const icons = {
    Chrome: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>',
    Firefox: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>',
    Safari: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M16.24 7.76l-1.41 1.41M9.17 14.83l-1.41 1.41M7.76 7.76l1.41 1.41M14.83 14.83l1.41 1.41" stroke="currentColor" stroke-width="1.5"/><polygon points="12,6 9,15 12,12 15,15" fill="currentColor"/></svg>',
    Edge: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M21 12c0 4.97-4.03 9-9 9-1.5 0-2.91-.37-4.15-1.02.25.02.5.02.75.02 3.31 0 6-2.69 6-6 0-2.49-1.52-4.63-3.68-5.54A8.03 8.03 0 0 1 21 12zM12 3c4.97 0 9 4.03 9 9 0 1.5-.37 2.91-1.02 4.15.02-.25.02-.5.02-.75 0-3.31-2.69-6-6-6-2.49 0-4.63 1.52-5.54 3.68A8.03 8.03 0 0 1 12 3z"/><circle cx="9" cy="15" r="4" fill="none" stroke="currentColor" stroke-width="2"/></svg>',
    Opera: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><ellipse cx="12" cy="12" rx="4" ry="8" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/></svg>',
    Brave: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L4 6v6c0 5.55 3.84 10.74 8 12 4.16-1.26 8-6.45 8-12V6l-8-4zm0 4l4 2v4c0 2.96-1.46 5.74-4 7.47-2.54-1.73-4-4.51-4-7.47V8l4-2z"/></svg>',
    Samsung: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M8 12h8M12 8v8" stroke="currentColor" stroke-width="2"/></svg>',
  };
  return icons[name] || "";
};

window.osIcon = function(name) {
  const icons = {
    Windows: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M3 12V6.5l7-1v6.5H3zm8-7.5V11h10V3L11 4.5zM3 13v5.5l7 1V13H3zm8 .5V19l10 2v-8H11z"/></svg>',
    macOS: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
    "Mac OS X": '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
    Linux: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12.5 2c-1.66 0-3 1.57-3 3.5 0 .66.15 1.27.41 1.81L8.04 9.19C7.12 8.75 6.09 8.5 5 8.5c-2.76 0-5 2.24-5 5s2.24 5 5 5c1.09 0 2.1-.35 2.93-.95l1.91 1.91c-.55.83-.84 1.79-.84 2.79 0 2.76 2.24 5 5 5s5-2.24 5-5c0-1-.29-1.96-.84-2.79l1.91-1.91c.83.6 1.84.95 2.93.95 2.76 0 5-2.24 5-5s-2.24-5-5-5c-1.09 0-2.12.25-3.04.69l-1.87-1.88c.26-.54.41-1.15.41-1.81 0-1.93-1.34-3.5-3-3.5zm0 2c.55 0 1 .67 1 1.5S13.05 7 12.5 7s-1-.67-1-1.5.45-1.5 1-1.5z"/></svg>',
    Android: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M6 18c0 .55.45 1 1 1h1v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h2v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h1c.55 0 1-.45 1-1V8H6v10zM3.5 8C2.67 8 2 8.67 2 9.5v7c0 .83.67 1.5 1.5 1.5S5 17.33 5 16.5v-7C5 8.67 4.33 8 3.5 8zm17 0c-.83 0-1.5.67-1.5 1.5v7c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5v-7c0-.83-.67-1.5-1.5-1.5zm-4.97-5.84l1.3-1.3c.2-.2.2-.51 0-.71-.2-.2-.51-.2-.71 0l-1.48 1.48A5.84 5.84 0 0 0 12 1c-.96 0-1.86.23-2.66.63L7.85.15c-.2-.2-.51-.2-.71 0-.2.2-.2.51 0 .71l1.31 1.31A5.983 5.983 0 0 0 6 7h12c0-1.99-.97-3.75-2.47-4.84zM10 5H9V4h1v1zm5 0h-1V4h1v1z"/></svg>',
    iOS: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
    "Chrome OS": '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>',
  };
  return icons[name] || "";
};

window.deviceIcon = function(type) {
  const icons = {
    desktop: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
    mobile: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>',
    tablet: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>',
  };
  return icons[type?.toLowerCase()] || "";
};

// Chart initialization - called via SSE ExecuteScript
window.initChart = function(labels, values) {
  const ctx = document.getElementById("pageviewsChart");
  if (!ctx) {
    console.error("Canvas element pageviewsChart not found");
    return;
  }

  // Skip if hidden
  if (ctx.offsetParent === null) {
    return;
  }

  // Skip if no data
  if (!labels || labels.length === 0) {
    if (pageviewsChart) {
      pageviewsChart.destroy();
      pageviewsChart = null;
    }
    return;
  }

  // Update existing chart
  if (pageviewsChart) {
    pageviewsChart.data.labels = labels;
    pageviewsChart.data.datasets[0].data = values;
    pageviewsChart.update("none");
    return;
  }

  // Create new chart
  pageviewsChart = new Chart(ctx, {
    type: "line",
    data: {
      labels: labels,
      datasets: [
        {
          label: "Pageviews",
          data: values,
          borderColor: "#3b82f6",
          backgroundColor: "rgba(59, 130, 246, 0.1)",
          fill: true,
          tension: 0.4,
          borderWidth: 2,
          pointRadius: 3,
          pointHoverRadius: 5,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: {
          mode: "index",
          intersect: false,
          backgroundColor: "rgba(0, 0, 0, 0.8)",
          padding: 12,
          titleFont: { size: 13 },
          bodyFont: { size: 14, weight: "bold" },
        },
      },
      scales: {
        y: {
          beginAtZero: true,
          ticks: { precision: 0, color: "#6b7280" },
          grid: { color: "#e5e7eb" },
        },
        x: {
          ticks: { color: "#6b7280", maxRotation: 0 },
          grid: { display: false },
        },
      },
      interaction: {
        mode: "nearest",
        axis: "x",
        intersect: false,
      },
    },
  });
};

// Destroy chart - cleanup
window.destroyChart = function() {
  if (pageviewsChart) {
    pageviewsChart.destroy();
    pageviewsChart = null;
  }
};
