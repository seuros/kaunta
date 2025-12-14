/**
 * Kaunta Websites Dashboard Alpine.js Component
 * Website management state and API interactions
 */
// eslint-disable-next-line no-unused-vars
function websitesDashboard() {
  return {
    websites: [],
    loading: true,
    error: null,
    showCreateModal: false,
    creating: false,
    createError: "",
    newWebsite: { domain: "", name: "" },
    expandedWebsite: null,
    addingDomainTo: null,
    newDomain: "",
    copiedId: null,
    toast: { show: false, message: "", type: "" },

    async init() {
      await this.loadWebsites();
    },

    async loadWebsites() {
      this.loading = true;
      this.error = null;
      try {
        const response = await fetch("/api/websites/list");
        if (!response.ok) {
          throw new Error("Failed to load websites");
        }
        this.websites = await response.json();
      } catch (err) {
        console.error("Failed to load websites:", err);
        this.error = err.message;
      } finally {
        this.loading = false;
      }
    },

    async createWebsite() {
      if (!this.newWebsite.domain) return;

      this.creating = true;
      this.createError = "";

      try {
        const response = await fetch("/api/websites", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify(this.newWebsite),
        });

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || "Failed to create website");
        }

        this.websites.push(data);
        this.showCreateModal = false;
        this.newWebsite = { domain: "", name: "" };
        this.showToast("Website created successfully!", "success");
      } catch (err) {
        this.createError = err.message;
      } finally {
        this.creating = false;
      }
    },

    async addDomain(website) {
      if (!this.newDomain) return;

      try {
        const response = await fetch(`/api/websites/${website.id}/domains`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify({ domain: this.newDomain }),
        });

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || "Failed to add domain");
        }

        // Update website in list
        const index = this.websites.findIndex((w) => w.id === website.id);
        if (index !== -1) {
          this.websites[index] = data;
        }

        this.addingDomainTo = null;
        this.newDomain = "";
        this.showToast("Domain added successfully!", "success");
      } catch (err) {
        this.showToast(err.message, "error");
      }
    },

    async removeDomain(website, domain) {
      if (website.allowed_domains.length <= 1) {
        this.showToast("Cannot remove the last domain", "error");
        return;
      }

      try {
        const response = await fetch(`/api/websites/${website.id}/domains`, {
          method: "DELETE",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify({ domain }),
        });

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || "Failed to remove domain");
        }

        // Update website in list
        const index = this.websites.findIndex((w) => w.id === website.id);
        if (index !== -1) {
          this.websites[index] = data;
        }

        this.showToast("Domain removed successfully!", "success");
      } catch (err) {
        this.showToast(err.message, "error");
      }
    },

    async togglePublicStats(website, enabled) {
      try {
        const response = await fetch(`/api/websites/${website.id}/public-stats`, {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify({ enabled }),
        });

        if (!response.ok) {
          const data = await response.json();
          throw new Error(data.error || "Failed to update public stats");
        }

        // Update local state
        const index = this.websites.findIndex((w) => w.id === website.id);
        if (index !== -1) {
          this.websites[index].public_stats_enabled = enabled;
        }

        this.showToast(
          enabled ? "Public stats enabled" : "Public stats disabled",
          "success"
        );
      } catch (err) {
        // Revert checkbox on error
        const index = this.websites.findIndex((w) => w.id === website.id);
        if (index !== -1) {
          this.websites[index].public_stats_enabled = !enabled;
        }
        this.showToast(err.message, "error");
      }
    },

    copyTrackingCode(website) {
      const code = `<script async src="/k.js" data-website-id="${website.id}"></script>`;
      navigator.clipboard
        .writeText(code)
        .then(() => {
          this.copiedId = website.id;
          this.showToast("Tracking code copied to clipboard!", "success");
          setTimeout(() => {
            this.copiedId = null;
          }, 2000);
        })
        .catch(() => {
          this.showToast("Failed to copy tracking code", "error");
        });
    },

    formatDate(dateStr) {
      if (!dateStr) return "";
      return new Date(dateStr).toLocaleDateString("en-US", {
        year: "numeric",
        month: "short",
        day: "numeric",
      });
    },

    showToast(message, type) {
      this.toast = { show: true, message, type };
      setTimeout(() => {
        this.toast.show = false;
      }, 3000);
    },

    getCsrfToken() {
      const value = "; " + document.cookie;
      const parts = value.split("; kaunta_csrf=");
      if (parts.length === 2) return parts.pop().split(";").shift();
      return "";
    },

    async logout() {
      try {
        const response = await fetch("/api/auth/logout", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
        });

        if (response.ok) {
          localStorage.removeItem("kaunta_website");
          localStorage.removeItem("kaunta_dateRange");
          window.location.href = "/login";
        } else {
          console.error("Logout failed:", await response.text());
          alert("Logout failed. Please try again.");
        }
      } catch (error) {
        console.error("Logout error:", error);
        alert("Network error during logout. Please try again.");
      }
    },
  };
}
