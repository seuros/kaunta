/**
 * Kaunta Goals Dashboard - Datastar Edition
 * Helper functions for Chart.js and goal analytics
 */

let goalChart = null;

window.formatGoalType = function(type) {
  return type === "page_view" ? "Page URL" : "Custom Event";
};

window.formatGoalDate = function(dateStr) {
  if (!dateStr) return "";
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

window.formatConversionRate = function(rate) {
  if (!rate) return "0%";
  return rate.toFixed(2) + "%";
};

window.getGoalsCsrfToken = function() {
  const value = "; " + document.cookie;
  const parts = value.split("; kaunta_csrf=");
  if (parts.length === 2) return parts.pop().split(";").shift();
  return "";
};

window.initGoalChart = function(labels, values, dateRange) {
  const ctx = document.getElementById("goalChart");
  if (!ctx) {
    console.error("Canvas element goalChart not found");
    return;
  }

  if (ctx.offsetParent === null) {
    return;
  }

  if (!labels || labels.length === 0) {
    if (goalChart) {
      goalChart.destroy();
      goalChart = null;
    }
    return;
  }

  const formattedLabels = labels.map((timestamp) => {
    const date = new Date(timestamp);
    if (dateRange === "1") {
      return date.toLocaleTimeString("en-US", {
        hour: "2-digit",
        minute: "2-digit",
      });
    } else if (dateRange === "7") {
      return date.toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
        hour: "2-digit",
      });
    } else {
      return date.toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
      });
    }
  });

  if (goalChart) {
    goalChart.data.labels = formattedLabels;
    goalChart.data.datasets[0].data = values;
    goalChart.update("none");
    return;
  }

  goalChart = new Chart(ctx, {
    type: "line",
    data: {
      labels: formattedLabels,
      datasets: [
        {
          label: "Completions",
          data: values,
          borderColor: "rgba(59, 130, 246, 1)",
          backgroundColor: "rgba(59, 130, 246, 0.1)",
          tension: 0.4,
          fill: true,
          borderWidth: 2,
          pointRadius: 3,
          pointHoverRadius: 5,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      plugins: {
        legend: {
          display: false,
        },
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
          ticks: {
            stepSize: 1,
            precision: 0,
            color: "#6b7280",
          },
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

window.destroyGoalChart = function() {
  if (goalChart) {
    goalChart.destroy();
    goalChart = null;
  }
};

window.confirmDeleteGoal = function(goalId, goalName) {
  return confirm(`Are you sure you want to delete the goal "${goalName}"?`);
};
