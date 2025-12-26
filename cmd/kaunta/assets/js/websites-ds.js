/**
 * Kaunta Websites Dashboard - Datastar Helper Functions
 * Utility functions for website management actions
 */

/**
 * Format a date string to localized display format
 * @param {string} dateStr - ISO date string
 * @returns {string} Formatted date
 */
function formatDate(dateStr) {
  if (!dateStr) return "";
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

/**
 * Copy tracking code snippet to clipboard
 * @param {string} websiteId - The website UUID
 * @param {object} signals - Datastar signals reference for updating copiedId
 */
async function copyTrackingCode(websiteId, signals) {
  const code = `<script async src="/k.js" data-website-id="${websiteId}"></script>`;
  try {
    await navigator.clipboard.writeText(code);
    signals.copiedId = websiteId;
    showToast("Tracking code copied to clipboard!", "success", signals);
    setTimeout(() => {
      signals.copiedId = null;
    }, 2000);
  } catch (err) {
    showToast("Failed to copy tracking code", "error", signals);
  }
}

/**
 * Show a toast notification
 * @param {string} message - Toast message
 * @param {string} type - Toast type (success, error)
 * @param {object} signals - Datastar signals reference
 */
function showToast(message, type, signals) {
  signals.toast = { show: true, message, type };
  setTimeout(() => {
    signals.toast = { show: false, message: "", type: "" };
  }, 3000);
}

/**
 * Get CSRF token from cookies
 * @returns {string} CSRF token value
 */
function getCsrfToken() {
  const value = "; " + document.cookie;
  const parts = value.split("; kaunta_csrf=");
  if (parts.length === 2) return parts.pop().split(";").shift();
  return "";
}

/**
 * Toggle expanded state for a website's domain list
 * @param {string} websiteId - The website UUID
 * @param {object} signals - Datastar signals reference
 */
function toggleExpanded(websiteId, signals) {
  signals.expandedWebsite = signals.expandedWebsite === websiteId ? null : websiteId;
}

/**
 * Reset the create modal state
 * @param {object} signals - Datastar signals reference
 */
function resetCreateModal(signals) {
  signals.showCreateModal = false;
  signals.createError = "";
  signals.newWebsite = { domain: "", name: "" };
}

/**
 * Cancel adding a domain
 * @param {object} signals - Datastar signals reference
 */
function cancelAddDomain(signals) {
  signals.addingDomainTo = null;
  signals.newDomain = "";
}

// Expose functions globally for Datastar actions
window.formatDate = formatDate;
window.copyTrackingCode = copyTrackingCode;
window.showToast = showToast;
window.getCsrfToken = getCsrfToken;
window.toggleExpanded = toggleExpanded;
window.resetCreateModal = resetCreateModal;
window.cancelAddDomain = cancelAddDomain;
