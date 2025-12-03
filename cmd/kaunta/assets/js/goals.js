/**
 * Kaunta Goals Dashboard Alpine.js Component
 * Goals state management and API interactions
 */
// eslint-disable-next-line no-unused-vars
function goalsDashboard() {
  return {
    websites: [],
    selectedWebsite: localStorage.getItem("kaunta_website") || "",
    goals: [],
    loading: true,
    goalsLoading: false,
    error: null,
    showCreateModal: false,
    showEditModal: false,
    submitting: false,
    formError: "",
    currentGoal: null,
    goalForm: {
      name: "",
      type: "",
      value: "",
    },
    toast: { show: false, message: "", type: "" },

    // Analytics modal state
    showAnalyticsModal: false,
    analyticsLoading: false,
    analyticsDateRange: "7",
    analyticsTab: "pages",
    analytics: {
      completions: 0,
      unique_sessions: 0,
      conversion_rate: 0,
      total_sessions: 0,
    },
    timeseriesData: [],
    breakdownData: [],
    goalChart: null,

    async init() {
      await this.loadWebsites();
      if (this.selectedWebsite) {
        await this.loadGoals();
      }
      this.loading = false;
    },

    async loadWebsites() {
      try {
        const response = await fetch("/api/websites");
        if (!response.ok) {
          throw new Error("Failed to load websites");
        }
        const data = await response.json();
        this.websites = Array.isArray(data.data)
          ? data.data
          : Array.isArray(data)
            ? data
            : [];

        if (!this.selectedWebsite && this.websites.length > 0) {
          this.selectedWebsite = this.websites[0].id;
          localStorage.setItem("kaunta_website", this.selectedWebsite);
        }
      } catch (err) {
        console.error("Failed to load websites:", err);
        this.error = err.message;
      }
    },

    async switchWebsite() {
      if (this.selectedWebsite) {
        localStorage.setItem("kaunta_website", this.selectedWebsite);
        this.goalsLoading = true;
        await this.loadGoals();
        this.goalsLoading = false;
      }
    },

    async loadGoals() {
      if (!this.selectedWebsite) return;

      this.goalsLoading = true;
      try {
        const response = await fetch(`/api/goals/${this.selectedWebsite}`);
        if (!response.ok) {
          throw new Error("Failed to load goals");
        }
        const data = await response.json();
        let responseData = data;
        if (responseData === null) responseData = [];
        this.goals = Array.isArray(responseData.data)
          ? responseData.data
          : Array.isArray(responseData)
            ? responseData
            : [];
      } catch (err) {
        console.error("Failed to load goals:", err);
        this.error = err.message;
        this.goals = [];
      } finally {
        this.goalsLoading = false;
      }
    },

    async createGoal() {
      if (!this.validateForm()) return;

      this.submitting = true;
      this.formError = "";

      try {
        const response = await fetch("/api/goals", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify({
            website_id: this.selectedWebsite,
            name: this.goalForm.name,
            type: this.goalForm.type,
            value: this.goalForm.value,
          }),
        });

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || "Failed to create goal");
        }

        this.goals.push(data);
        this.closeModals();
        this.showToast("Goal created successfully!", "success");
      } catch (err) {
        this.formError = err.message;
      } finally {
        this.submitting = false;
      }
    },

    editGoal(goal) {
      this.currentGoal = goal;
      this.goalForm = {
        name: goal.name,
        type: goal.type,
        value: goal.value,
      };
      this.showEditModal = true;
    },

    async updateGoal() {
      if (!this.validateForm() || !this.currentGoal) return;

      this.submitting = true;
      this.formError = "";

      try {
        const response = await fetch(`/api/goals/${this.currentGoal.id}`, {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
          body: JSON.stringify({
            name: this.goalForm.name,
            type: this.goalForm.type,
            value: this.goalForm.value,
          }),
        });

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || "Failed to update goal");
        }

        const index = this.goals.findIndex((g) => g.id === this.currentGoal.id);
        if (index !== -1) {
          this.goals[index] = data;
        }

        this.closeModals();
        this.showToast("Goal updated successfully!", "success");
      } catch (err) {
        this.formError = err.message;
      } finally {
        this.submitting = false;
      }
    },

    async deleteGoal(goal) {
      if (
        !confirm(`Are you sure you want to delete the goal "${goal.name}"?`)
      ) {
        return;
      }

      try {
        const response = await fetch(`/api/goals/${goal.id}`, {
          method: "DELETE",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": this.getCsrfToken(),
          },
          credentials: "include",
        });

        if (!response.ok) {
          const data = await response.json();
          throw new Error(data.error || "Failed to delete goal");
        }

        this.goals = this.goals.filter((g) => g.id !== goal.id);
        this.showToast("Goal deleted successfully!", "success");
      } catch (err) {
        this.showToast(err.message, "error");
      }
    },

    validateForm() {
      if (!this.goalForm.name.trim()) {
        this.formError = "Goal name is required";
        return false;
      }
      if (!this.goalForm.type) {
        this.formError = "Goal type is required";
        return false;
      }
      if (!this.goalForm.value.trim()) {
        this.formError = "Goal value is required";
        return false;
      }
      return true;
    },

    onTypeChange() {
      this.goalForm.value = "";
      this.formError = "";
    },

    closeModals() {
      this.showCreateModal = false;
      this.showEditModal = false;
      this.currentGoal = null;
      this.goalForm = { name: "", type: "", value: "" };
      this.formError = "";
    },

    getWebsiteName() {
      if (!this.selectedWebsite || !this.websites.length) return "";
      const website = this.websites.find((w) => w.id === this.selectedWebsite);
      return website ? website.name || website.domain : "";
    },

    formatType(type) {
      return type === "page_view" ? "Page URL" : "Custom Event";
    },

    getTypeClass(type) {
      return type === "page_view" ? "page" : "event";
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

    // Analytics Modal Functions
    async viewAnalytics(goal) {
      this.currentGoal = goal;
      this.showAnalyticsModal = true;
      this.analyticsDateRange = "7";
      this.analyticsTab = "pages";
      await this.loadGoalAnalytics();
    },

    async loadGoalAnalytics() {
      if (!this.currentGoal) return;

      this.analyticsLoading = true;

      try {
        const [analyticsRes, timeseriesRes] = await Promise.all([
          fetch(
            `/api/dashboard/goals/${this.currentGoal.id}/analytics?days=${this.analyticsDateRange}`,
          ),
          fetch(
            `/api/dashboard/goals/${this.currentGoal.id}/timeseries?days=${this.analyticsDateRange}`,
          ),
        ]);

        if (!analyticsRes.ok || !timeseriesRes.ok) {
          throw new Error("Failed to load analytics data");
        }

        this.analytics = await analyticsRes.json();
        this.timeseriesData = await timeseriesRes.json();

        this.renderGoalChart();
        await this.loadBreakdown();
      } catch (err) {
        console.error("Failed to load goal analytics:", err);
        this.showToast("Failed to load analytics", "error");
      } finally {
        this.analyticsLoading = false;
      }
    },

    async loadBreakdown() {
      if (!this.currentGoal) return;

      try {
        let url;
        if (this.analyticsTab === "pages") {
          url = `/api/dashboard/goals/${this.currentGoal.id}/converting-pages?days=${this.analyticsDateRange}&per=10&offset=0`;
        } else {
          url = `/api/dashboard/goals/${this.currentGoal.id}/breakdown/${this.analyticsTab}?days=${this.analyticsDateRange}&per=10&offset=0`;
        }

        const response = await fetch(url);
        if (!response.ok) {
          throw new Error("Failed to load breakdown");
        }

        const data = await response.json();
        this.breakdownData = Array.isArray(data.data)
          ? data.data
          : Array.isArray(data)
            ? data
            : [];
      } catch (err) {
        console.error("Failed to load breakdown:", err);
        this.breakdownData = [];
      }
    },

    setAnalyticsDateRange(days) {
      this.analyticsDateRange = days;
      this.loadGoalAnalytics();
    },

    setAnalyticsTab(tab) {
      this.analyticsTab = tab;
      this.loadBreakdown();
    },

    renderGoalChart() {
      if (this.goalChart) {
        this.goalChart.destroy();
        this.goalChart = null;
      }

      const canvas = document.getElementById("goalChart");
      if (!canvas) return;

      const ctx = canvas.getContext("2d");

      const labels = this.timeseriesData.map((point) => {
        const date = new Date(point.timestamp);
        if (this.analyticsDateRange === "1") {
          return date.toLocaleTimeString("en-US", {
            hour: "2-digit",
            minute: "2-digit",
          });
        } else if (this.analyticsDateRange === "7") {
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

      const values = this.timeseriesData.map((point) => point.value);

      this.goalChart = new Chart(ctx, {
        type: "line",
        data: {
          labels: labels,
          datasets: [
            {
              label: "Completions",
              data: values,
              borderColor: "rgba(59, 130, 246, 1)",
              backgroundColor: "rgba(59, 130, 246, 0.1)",
              tension: 0.4,
              fill: true,
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
          },
          scales: {
            y: {
              beginAtZero: true,
              ticks: {
                stepSize: 1,
              },
            },
          },
        },
      });
    },

    closeAnalyticsModal() {
      this.showAnalyticsModal = false;
      this.currentGoal = null;
      if (this.goalChart) {
        this.goalChart.destroy();
        this.goalChart = null;
      }
      this.breakdownData = [];
      this.timeseriesData = [];
    },
  };
}
