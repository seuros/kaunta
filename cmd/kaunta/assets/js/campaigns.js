/**
 * Kaunta Campaigns Dashboard Alpine.js Component
 * UTM campaign tracking and analytics state management
 */
// eslint-disable-next-line no-unused-vars
function campaignsDashboard() {
  return {
    websites: [],
    websitesLoading: true,
    websitesError: false,
    selectedWebsite: localStorage.getItem("kaunta_website") || "",
    initialized: false,
    data: {
      source: [],
      medium: [],
      campaign: [],
      term: [],
      content: [],
    },
    loading: {
      source: false,
      medium: false,
      campaign: false,
      term: false,
      content: false,
    },
    sort: {
      source: { column: "count", direction: "desc" },
      medium: { column: "count", direction: "desc" },
      campaign: { column: "count", direction: "desc" },
      term: { column: "count", direction: "desc" },
      content: { column: "count", direction: "desc" },
    },

    async init() {
      if (this.initialized) return;
      this.initialized = true;
      await this.loadWebsites();
    },

    async loadWebsites() {
      this.websitesLoading = true;
      this.websitesError = false;
      try {
        const response = await fetch("/api/websites");
        if (!response.ok) {
          this.websitesError = true;
          return;
        }
        const response_data = await response.json();
        const sites = response_data.data || response_data;
        this.websites = Array.isArray(sites) ? sites : [];
        const hasStoredSelection =
          this.selectedWebsite &&
          this.websites.some((site) => site.id === this.selectedWebsite);
        if (!hasStoredSelection && this.selectedWebsite) {
          this.selectedWebsite = "";
          localStorage.removeItem("kaunta_website");
        }
        if (!this.selectedWebsite && this.websites.length > 0) {
          this.selectedWebsite = this.websites[0].id;
          localStorage.setItem("kaunta_website", this.selectedWebsite);
        }

        // Load UTM data after website is determined
        if (this.selectedWebsite) {
          await this.loadAllUTMData();
        }
      } catch (error) {
        console.error("Failed to load websites:", error);
        this.websitesError = true;
      } finally {
        this.websitesLoading = false;
      }
    },

    async loadAllUTMData() {
      // Load all UTM dimensions in parallel
      await Promise.all([
        this.loadUTMData("source"),
        this.loadUTMData("medium"),
        this.loadUTMData("campaign"),
        this.loadUTMData("term"),
        this.loadUTMData("content"),
      ]);
    },

    async loadUTMData(dimension) {
      if (!this.selectedWebsite) return;
      this.loading[dimension] = true;
      try {
        const sortState = this.sort[dimension];
        const params = new URLSearchParams({
          sort_by: sortState.column,
          sort_order: sortState.direction,
        });
        const response = await fetch(
          `/api/dashboard/utm-${dimension}/${this.selectedWebsite}?${params}`,
        );
        if (response.ok) {
          const result = await response.json();
          this.data[dimension] = result.data || [];
        }
      } catch (error) {
        console.error(`Failed to load UTM ${dimension}:`, error);
      } finally {
        this.loading[dimension] = false;
      }
    },

    async sortBy(dimension, column) {
      const sortState = this.sort[dimension];
      if (sortState.column === column) {
        sortState.direction = sortState.direction === "asc" ? "desc" : "asc";
      } else {
        sortState.column = column;
        sortState.direction = "desc";
      }
      await this.loadUTMData(dimension);
    },

    async logout() {
      try {
        const csrfToken = this.getCsrfToken();
        const response = await fetch("/api/auth/logout", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": csrfToken,
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

    getCsrfToken() {
      const value = "; " + document.cookie;
      const parts = value.split("; kaunta_csrf=");
      if (parts.length === 2) return parts.pop().split(";").shift();
      return "";
    },
  };
}
