/**
 * Kaunta Dashboard Alpine.js Component
 * Analytics dashboard state management and API interactions
 */
// eslint-disable-next-line no-unused-vars
function dashboard() {
  return {
    websites: [],
    websitesLoading: true,
    websitesError: false,
    selectedWebsite: localStorage.getItem("kaunta_website") || "",
    dateRange: localStorage.getItem("kaunta_dateRange") || "1",
    stats: {
      current_visitors: 0,
      today_pageviews: 0,
      today_visitors: 0,
      today_bounce_rate: "0%",
    },
    pages: [],
    loading: true,
    sortColumn: "count",
    sortDirection: "desc",
    refreshInterval: null,
    chartRefreshInterval: null,
    realtimeSocket: null,
    realtimeReconnectTimer: null,
    realtimeRefreshTimeout: null,
    chart: null,
    activeTab: "pages",
    breakdownData: [],
    breakdownLoading: false,
    filters: {
      country: "",
      browser: "",
      device: "",
      page: "",
    },
    availableFilters: {
      countries: [],
      browsers: [],
      devices: [],
      pages: [],
    },
    mapLoading: false,
    mapData: null,
    mapInitialized: false,

    // Icon helper: Country code to flag emoji
    countryToFlag(code) {
      if (!code || code.length !== 2) return "";
      const codePoints = code
        .toUpperCase()
        .split("")
        .map((char) => 127397 + char.charCodeAt(0));
      return String.fromCodePoint(...codePoints);
    },

    // Icon helper: Browser name to inline SVG
    browserIcon(name) {
      const icons = {
        Chrome:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>',
        Firefox:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>',
        Safari:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M16.24 7.76l-1.41 1.41M9.17 14.83l-1.41 1.41M7.76 7.76l1.41 1.41M14.83 14.83l1.41 1.41" stroke="currentColor" stroke-width="1.5"/><polygon points="12,6 9,15 12,12 15,15" fill="currentColor"/></svg>',
        Edge: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M21 12c0 4.97-4.03 9-9 9-1.5 0-2.91-.37-4.15-1.02.25.02.5.02.75.02 3.31 0 6-2.69 6-6 0-2.49-1.52-4.63-3.68-5.54A8.03 8.03 0 0 1 21 12zM12 3c4.97 0 9 4.03 9 9 0 1.5-.37 2.91-1.02 4.15.02-.25.02-.5.02-.75 0-3.31-2.69-6-6-6-2.49 0-4.63 1.52-5.54 3.68A8.03 8.03 0 0 1 12 3z"/><circle cx="9" cy="15" r="4" fill="none" stroke="currentColor" stroke-width="2"/></svg>',
        Opera:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><ellipse cx="12" cy="12" rx="4" ry="8" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/></svg>',
        Brave:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L4 6v6c0 5.55 3.84 10.74 8 12 4.16-1.26 8-6.45 8-12V6l-8-4zm0 4l4 2v4c0 2.96-1.46 5.74-4 7.47-2.54-1.73-4-4.51-4-7.47V8l4-2z"/></svg>',
        Samsung:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M8 12h8M12 8v8" stroke="currentColor" stroke-width="2"/></svg>',
      };
      return icons[name] || "";
    },

    // Icon helper: OS name to inline SVG
    osIcon(name) {
      const icons = {
        Windows:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M3 12V6.5l7-1v6.5H3zm8-7.5V11h10V3L11 4.5zM3 13v5.5l7 1V13H3zm8 .5V19l10 2v-8H11z"/></svg>',
        macOS:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
        "Mac OS X":
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
        Linux:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12.5 2c-1.66 0-3 1.57-3 3.5 0 .66.15 1.27.41 1.81L8.04 9.19C7.12 8.75 6.09 8.5 5 8.5c-2.76 0-5 2.24-5 5s2.24 5 5 5c1.09 0 2.1-.35 2.93-.95l1.91 1.91c-.55.83-.84 1.79-.84 2.79 0 2.76 2.24 5 5 5s5-2.24 5-5c0-1-.29-1.96-.84-2.79l1.91-1.91c.83.6 1.84.95 2.93.95 2.76 0 5-2.24 5-5s-2.24-5-5-5c-1.09 0-2.12.25-3.04.69l-1.87-1.88c.26-.54.41-1.15.41-1.81 0-1.93-1.34-3.5-3-3.5zm0 2c.55 0 1 .67 1 1.5S13.05 7 12.5 7s-1-.67-1-1.5.45-1.5 1-1.5z"/></svg>',
        Android:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M6 18c0 .55.45 1 1 1h1v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h2v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h1c.55 0 1-.45 1-1V8H6v10zM3.5 8C2.67 8 2 8.67 2 9.5v7c0 .83.67 1.5 1.5 1.5S5 17.33 5 16.5v-7C5 8.67 4.33 8 3.5 8zm17 0c-.83 0-1.5.67-1.5 1.5v7c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5v-7c0-.83-.67-1.5-1.5-1.5zm-4.97-5.84l1.3-1.3c.2-.2.2-.51 0-.71-.2-.2-.51-.2-.71 0l-1.48 1.48A5.84 5.84 0 0 0 12 1c-.96 0-1.86.23-2.66.63L7.85.15c-.2-.2-.51-.2-.71 0-.2.2-.2.51 0 .71l1.31 1.31A5.983 5.983 0 0 0 6 7h12c0-1.99-.97-3.75-2.47-4.84zM10 5H9V4h1v1zm5 0h-1V4h1v1z"/></svg>',
        iOS: '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>',
        "Chrome OS":
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>',
      };
      return icons[name] || "";
    },

    // Icon helper: Device type to inline SVG
    deviceIcon(type) {
      const icons = {
        desktop:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
        mobile:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>',
        tablet:
          '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>',
      };
      return icons[type?.toLowerCase()] || "";
    },

    async init() {
      // Load filters from URL parameters
      const urlParams = new URLSearchParams(window.location.search);
      if (urlParams.get("country")) this.filters.country = urlParams.get("country");
      if (urlParams.get("browser")) this.filters.browser = urlParams.get("browser");
      if (urlParams.get("device")) this.filters.device = urlParams.get("device");
      if (urlParams.get("page")) this.filters.page = urlParams.get("page");

      await this.loadWebsites();
      if (this.websitesError) {
        this.loading = false;
        return;
      }

      if (this.selectedWebsite) {
        // Sequential loading to prevent race conditions
        await this.loadAvailableFilters();
        await this.loadStats();
        await this.loadTopPages();
        await this.loadBreakdown();

        // Chart loading is now handled by x-intersect on the chart element itself.
        // Set up refresh intervals
        this.refreshInterval = setInterval(() => this.loadStats(), 5000);
        setInterval(() => this.loadTopPages(), 30000);
        setInterval(() => this.loadBreakdown(), 30000);
        this.connectRealtime();
      }
    },

    async loadChartAndSetupIntervals() {
      // Clear existing interval to prevent duplicates
      if (this.chartRefreshInterval) {
        clearInterval(this.chartRefreshInterval);
      }
      await this.loadChart();
      this.chartRefreshInterval = setInterval(() => {
        // Only refresh if canvas is visible (not hidden by x-show)
        const canvas = document.getElementById("pageviewsChart");
        if (canvas && canvas.offsetParent !== null) {
          this.loadChart();
        }
      }, 60000);
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
        // Handle paginated response format: {data: [...], pagination: {...}}
        const sites = response_data.data || response_data;
        this.websites = Array.isArray(sites) ? sites : [];
        const hasStoredSelection =
          this.selectedWebsite && this.websites.some((site) => site.id === this.selectedWebsite);
        if (!hasStoredSelection && this.selectedWebsite) {
          this.selectedWebsite = "";
          localStorage.removeItem("kaunta_website");
        }
        if (!this.selectedWebsite && this.websites.length > 0) {
          this.selectedWebsite = this.websites[0].id;
          localStorage.setItem("kaunta_website", this.selectedWebsite);
        }
      } catch (error) {
        console.error("Failed to load websites:", error);
        this.websitesError = true;
      } finally {
        this.websitesLoading = false;
      }
    },

    async switchWebsite() {
      if (this.selectedWebsite) {
        localStorage.setItem("kaunta_website", this.selectedWebsite);
        this.loading = true;

        // Clear existing intervals
        if (this.refreshInterval) {
          clearInterval(this.refreshInterval);
        }
        if (this.chartRefreshInterval) {
          clearInterval(this.chartRefreshInterval);
        }

        // Sequential loading to prevent race conditions
        await this.loadAvailableFilters();
        await this.loadStats();
        await this.loadTopPages();
        await this.loadBreakdown();

        // Wait for DOM update before chart
        await this.$nextTick();
        await this.loadChart();
        await this.loadMapData();

        this.refreshInterval = setInterval(() => this.loadStats(), 5000);
        this.connectRealtime(true);
      }
    },

    connectRealtime(forceReconnect = false) {
      if (
        typeof WebSocket === "undefined" ||
        !this.selectedWebsite ||
        (this.realtimeSocket && !forceReconnect)
      ) {
        return;
      }

      if (this.realtimeSocket) {
        this.realtimeSocket.close();
        this.realtimeSocket = null;
      }

      if (this.realtimeReconnectTimer) {
        clearTimeout(this.realtimeReconnectTimer);
        this.realtimeReconnectTimer = null;
      }

      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const wsUrl = `${protocol}://${window.location.host}/ws/realtime`;

      try {
        const socket = new WebSocket(wsUrl);
        this.realtimeSocket = socket;

        socket.onmessage = (event) => {
          try {
            const payload = JSON.parse(event.data);
            if (payload.website_id && payload.website_id === this.selectedWebsite) {
              this.scheduleRealtimeRefresh();
            }
          } catch (error) {
            console.warn("Realtime payload parse error:", error);
          }
        };

        socket.onclose = () => {
          this.realtimeSocket = null;
          this.realtimeReconnectTimer = setTimeout(() => this.connectRealtime(), 5000);
        };

        socket.onerror = () => {
          socket.close();
        };
      } catch (error) {
        console.warn("Realtime connection error:", error);
        this.realtimeReconnectTimer = setTimeout(() => this.connectRealtime(), 5000);
      }
    },

    scheduleRealtimeRefresh() {
      if (this.realtimeRefreshTimeout) {
        return;
      }
      this.realtimeRefreshTimeout = setTimeout(async () => {
        await this.loadStats();
        this.realtimeRefreshTimeout = null;
      }, 750);
    },

    buildFilterParams(prefix = "") {
      const params = new URLSearchParams();
      if (this.filters.country) params.set("country", this.filters.country);
      if (this.filters.browser) params.set("browser", this.filters.browser);
      if (this.filters.device) params.set("device", this.filters.device);
      if (this.filters.page) params.set("page", this.filters.page);
      const queryString = params.toString();
      return queryString ? prefix + queryString : "";
    },

    async setDateRange(range) {
      this.dateRange = range;
      localStorage.setItem("kaunta_dateRange", range);

      // Sequential loading to prevent race conditions
      await this.loadStats();
      await this.loadTopPages();
      await this.loadBreakdown();

      await this.$nextTick();
      await this.loadChart();
      await this.loadMapData();
    },

    async loadStats() {
      if (!this.selectedWebsite) return;
      try {
        const filterParams = this.buildFilterParams("?");
        const response = await fetch(
          `/api/dashboard/stats/${this.selectedWebsite}${filterParams}`
        );
        if (response.ok) {
          this.stats = await response.json();
        }
      } catch (error) {
        console.error("Failed to load stats:", error);
      }
    },

    async loadTopPages() {
      if (!this.selectedWebsite) return;
      try {
        const filterParams = this.buildFilterParams("&");
        const response = await fetch(
          `/api/dashboard/pages/${this.selectedWebsite}?limit=10${filterParams}`
        );
        if (response.ok) {
          this.pages = await response.json();
          this.loading = false;
        }
      } catch (error) {
        console.error("Failed to load top pages:", error);
        this.loading = false;
      }
    },

    async loadChart() {
      if (!this.selectedWebsite) return;
      try {
        const days = this.dateRange === "1" ? 1 : this.dateRange === "7" ? 7 : 30;
        const filterParams = this.buildFilterParams("&");
        const response = await fetch(
          `/api/dashboard/timeseries/${this.selectedWebsite}?days=${days}${filterParams}`
        );
        if (response.ok) {
          const data = await response.json();

          // Handle empty data
          const labels =
            data && data.length > 0
              ? data.map((point) => {
                  const date = new Date(point.timestamp);
                  return days === 1
                    ? date.toLocaleTimeString("en-US", {
                        hour: "numeric",
                        hour12: true,
                      })
                    : date.toLocaleDateString("en-US", {
                        month: "short",
                        day: "numeric",
                      });
                })
              : [];
          const values = data && data.length > 0 ? data.map((point) => point.value) : [];
          const ctx = document.getElementById("pageviewsChart");
          if (!ctx) {
            console.error("Canvas element pageviewsChart not found");
            return;
          }
          // Check if canvas is visible (not hidden by x-show)
          if (ctx.offsetParent === null) {
            return;
          }

          // Skip chart creation if no data to prevent Chart.js fill errors
          if (labels.length === 0) {
            if (this.chart) {
              this.chart.destroy();
              this.chart = null;
            }
            return;
          }

          // Update existing chart data instead of recreating (more performant)
          if (this.chart) {
            this.chart.data.labels = labels;
            this.chart.data.datasets[0].data = values;
            this.chart.update("none"); // 'none' skips animations for faster updates
            return;
          }

          this.chart = new Chart(ctx, {
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
                    precision: 0,
                    color: "#6b7280",
                  },
                  grid: {
                    color: "#e5e7eb",
                  },
                },
                x: {
                  ticks: {
                    color: "#6b7280",
                    maxRotation: 0,
                  },
                  grid: {
                    display: false,
                  },
                },
              },
              interaction: {
                mode: "nearest",
                axis: "x",
                intersect: false,
              },
            },
          });
        }
      } catch (error) {
        console.error("Failed to load chart:", error);
        // Try to initialize chart again after a short delay
        setTimeout(() => {
          const ctx = document.getElementById("pageviewsChart");
          if (ctx && ctx.offsetParent !== null && !this.chart) {
            console.log("Retrying chart initialization...");
            this.loadChart();
          }
        }, 100);
      }
    },

    async loadBreakdown() {
      if (!this.selectedWebsite || this.activeTab === "map") {
        return;
      }
      this.breakdownLoading = true;
      // Reset data to prevent showing stale data
      this.breakdownData = [];

      try {
        const endpoint = this.activeTab;
        // Build URL with filters and sorting
        const params = new URLSearchParams();
        if (this.filters.country) params.set("country", this.filters.country);
        if (this.filters.browser) params.set("browser", this.filters.browser);
        if (this.filters.device) params.set("device", this.filters.device);
        if (this.filters.page) params.set("page", this.filters.page);
        params.set("sort_by", this.sortColumn);
        params.set("sort_order", this.sortDirection);
        const queryString = params.toString();
        const response = await fetch(
          `/api/dashboard/${endpoint}/${this.selectedWebsite}?${queryString}`
        );
        if (response.ok) {
          const result = await response.json();
          const data = result.data || [];
          // Ensure data is valid and has required fields
          this.breakdownData = Array.isArray(data)
            ? data.map((item, index) => ({
                ...item,
                name: item.name || item.path || item.country_name || `Item ${index + 1}`,
                count: item.count || item.views || item.visitors || 0,
              }))
            : [];
        } else {
          this.breakdownData = [];
        }
      } catch (error) {
        console.error("Failed to load breakdown:", error);
        this.breakdownData = [];
      } finally {
        this.breakdownLoading = false;
      }
    },

    async loadAvailableFilters() {
      if (!this.selectedWebsite) return;
      try {
        const countriesRes = await fetch(
          `/api/dashboard/countries/${this.selectedWebsite}?limit=100`
        );
        if (countriesRes.ok) {
          this.availableFilters.countries = (await countriesRes.json()).data;
        }
        const browsersRes = await fetch(
          `/api/dashboard/browsers/${this.selectedWebsite}?limit=100`
        );
        if (browsersRes.ok) {
          this.availableFilters.browsers = (await browsersRes.json()).data;
        }
        const devicesRes = await fetch(
          `/api/dashboard/devices/${this.selectedWebsite}?limit=100`
        );
        if (devicesRes.ok) {
          this.availableFilters.devices = (await devicesRes.json()).data;
        }
        const pagesRes = await fetch(
          `/api/dashboard/pages/${this.selectedWebsite}?limit=100`
        );
        if (pagesRes.ok) {
          this.availableFilters.pages = (await pagesRes.json()).data;
        }
      } catch (error) {
        console.error("Failed to load filter options:", error);
      }
    },

    async applyFilter() {
      this.updateURL();

      // Sequential loading to prevent race conditions
      await this.loadStats();
      await this.loadTopPages();
      await this.loadBreakdown();

      await this.$nextTick();
      await this.loadChart();
      await this.loadMapData();
    },

    clearFilters() {
      this.filters = {
        country: "",
        browser: "",
        device: "",
        page: "",
      };
      this.applyFilter();
    },

    updateURL() {
      const params = new URLSearchParams();
      if (this.filters.country) params.set("country", this.filters.country);
      if (this.filters.browser) params.set("browser", this.filters.browser);
      if (this.filters.device) params.set("device", this.filters.device);
      if (this.filters.page) params.set("page", this.filters.page);
      const newURL = params.toString()
        ? `${window.location.pathname}?${params.toString()}`
        : window.location.pathname;
      window.history.pushState({}, "", newURL);
    },

    get hasActiveFilters() {
      return (
        this.filters.country || this.filters.browser || this.filters.device || this.filters.page
      );
    },

    async sortBy(column) {
      if (this.sortColumn === column) {
        this.sortDirection = this.sortDirection === "asc" ? "desc" : "asc";
      } else {
        this.sortColumn = column;
        this.sortDirection = "desc";
      }
      // Reload data with new sort order
      await this.loadBreakdown();
    },

    async loadMapData() {
      if (!this.selectedWebsite || this.activeTab !== "map") return;
      this.mapLoading = true;
      try {
        const days = this.dateRange === "1" ? 1 : this.dateRange === "7" ? 7 : 30;
        const params = new URLSearchParams({ days });
        if (this.filters.country) params.append("country", this.filters.country);
        if (this.filters.browser) params.append("browser", this.filters.browser);
        if (this.filters.device) params.append("device", this.filters.device);
        if (this.filters.page) params.append("page", this.filters.page);
        const response = await fetch(`/api/dashboard/map/${this.selectedWebsite}?${params}`);
        if (response.ok) {
          this.mapData = await response.json();
          const containerExists = document.getElementById("choropleth-map");
          if (!containerExists) {
            console.warn("Choropleth map container not found yet; will retry once rendered.");
            return;
          }
          if (!this.mapInitialized) {
            await this.initializeChoropleth();
          } else {
            this.updateChoropleth();
          }
        } else {
          console.error("Failed to load map data");
        }
      } catch (error) {
        console.error("Map data error:", error);
      } finally {
        this.mapLoading = false;
      }
    },

    getColorForValue(value, maxValue) {
      if (!value || value === 0) return "#e5e5e5";
      const intensity = value / maxValue;
      if (intensity < 0.2) return "#deebf7";
      if (intensity < 0.4) return "#9ecae1";
      if (intensity < 0.6) return "#6baed6";
      if (intensity < 0.8) return "#3182bd";
      return "#08519c";
    },

    formatTooltipText(countryName, visitors, percentage) {
      return `
        <div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
          <div style="font-weight: 500; font-size: 14px; margin-bottom: 4px; color: #1a1a1a;">${countryName}</div>
          <div style="font-size: 13px; color: #666666;">
            ${visitors.toLocaleString()} visitors (${percentage.toFixed(1)}%)
          </div>
        </div>
      `;
    },

    handleCountryClick(countryName) {
      this.filters.country = countryName;
      this.applyFilter();
      this.activeTab = "countries";
      this.loadBreakdown();
    },

    async initializeChoropleth() {
      try {
        const container = document.getElementById("choropleth-map");
        if (!container) {
          console.error("Choropleth map container not found");
          return;
        }
        const map = L.map("choropleth-map", {
          zoomControl: true,
          attributionControl: true,
          scrollWheelZoom: true,
          doubleClickZoom: true,
          touchZoom: true,
        }).setView([20, 0], 2);
        L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
          attribution:
            '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
          maxZoom: 18,
          minZoom: 1,
        }).addTo(map);
        const response = await fetch("/assets/data/countries-110m.json");
        if (!response.ok) {
          throw new Error(`Failed to load TopoJSON: ${response.statusText}`);
        }
        const world = await response.json();
        const features = topojson.feature(world, world.objects.countries).features;
        const dataMap = new Map();
        let maxValue = 0;
        if (this.mapData && this.mapData.data && Array.isArray(this.mapData.data)) {
          maxValue = Math.max(...this.mapData.data.map((d) => d.visitors || 0));
          this.mapData.data.forEach((d) => {
            dataMap.set(d.country_name, {
              visitors: d.visitors || 0,
              percentage: d.percentage || 0,
              name: d.country_name,
              code: d.code,
            });
            if (d.code) {
              dataMap.set(d.code, {
                visitors: d.visitors || 0,
                percentage: d.percentage || 0,
                name: d.country_name,
                code: d.code,
              });
            }
          });
        }
        const getStyle = (feature) => {
          const countryName = feature.properties.name;
          const countryId = feature.id;
          const countryData = dataMap.get(countryName) || dataMap.get(countryId);
          const fillColor = countryData
            ? this.getColorForValue(countryData.visitors, maxValue)
            : "#e5e5e5";
          return {
            fillColor: fillColor,
            weight: 0.5,
            opacity: 0.8,
            color: "#999",
            fillOpacity: 0.7,
          };
        };
        const onEachFeature = (feature, layer) => {
          const countryName = feature.properties.name;
          const countryId = feature.id;
          const countryData = dataMap.get(countryName) || dataMap.get(countryId);
          const tooltipContent = countryData
            ? this.formatTooltipText(countryData.name, countryData.visitors, countryData.percentage)
            : `<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
                 <div style="font-weight: 500; font-size: 14px; color: #1a1a1a;">${countryName || "Unknown"}</div>
                 <div style="font-size: 13px; color: #666666;">No visitors</div>
               </div>`;
          layer.bindTooltip(tooltipContent, {
            sticky: true,
            opacity: 0.95,
            className: "leaflet-custom-tooltip",
          });
          layer.on("mouseover", function (e) {
            const layer = e.target;
            layer.setStyle({
              weight: 2,
              color: "#333",
              fillOpacity: 0.85,
            });
            if (!L.Browser.ie && !L.Browser.opera && !L.Browser.edge) {
              layer.bringToFront();
            }
          });
          layer.on("mouseout", function (e) {
            const layer = e.target;
            layer.setStyle({
              weight: 0.5,
              color: "#999",
              fillOpacity: 0.7,
            });
          });
          if (countryData) {
            layer.on("click", () => {
              this.handleCountryClick(countryData.name);
            });
            layer.on("mouseover", function () {
              container.style.cursor = "pointer";
            });
            layer.on("mouseout", function () {
              container.style.cursor = "";
            });
          }
        };
        const geoJsonLayer = L.geoJSON(features, {
          style: getStyle,
          onEachFeature: onEachFeature,
        }).addTo(map);
        this.mapInstance = map;
        this.geoJsonLayer = geoJsonLayer;
        if (!document.getElementById("leaflet-custom-tooltip-style")) {
          const style = document.createElement("style");
          style.id = "leaflet-custom-tooltip-style";
          style.textContent = `
            .leaflet-custom-tooltip {
              background: rgba(255, 255, 255, 0.95);
              border: 1px solid #e5e7eb;
              border-radius: 8px;
              box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
              padding: 12px;
            }
            .leaflet-custom-tooltip::before {
              border-top-color: rgba(255, 255, 255, 0.95);
            }
          `;
          document.head.appendChild(style);
        }
        this.mapInitialized = true;
      } catch (error) {
        console.error("Failed to initialize choropleth map:", error);
        const container = document.getElementById("choropleth-map");
        if (container) {
          container.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: center; height: 100%; color: #999999;">
              <div style="text-align: center;">
                <div style="font-size: 48px; margin-bottom: 16px; opacity: 0.3;">🗺️</div>
                <div style="font-size: 16px; font-weight: 500; color: #1f2937; margin-bottom: 8px;">Map Loading Error</div>
                <div style="font-size: 14px;">Failed to load map data. Please try again.</div>
              </div>
            </div>
          `;
        }
      }
    },

    updateChoropleth() {
      try {
        if (!this.mapInstance || !this.geoJsonLayer) {
          console.warn("Map not initialized, cannot update");
          return;
        }
        const dataMap = new Map();
        let maxValue = 0;
        if (this.mapData && this.mapData.data && Array.isArray(this.mapData.data)) {
          maxValue = Math.max(...this.mapData.data.map((d) => d.visitors || 0));
          this.mapData.data.forEach((d) => {
            dataMap.set(d.country_name, {
              visitors: d.visitors || 0,
              percentage: d.percentage || 0,
              name: d.country_name,
              code: d.code,
            });
            if (d.code) {
              dataMap.set(d.code, {
                visitors: d.visitors || 0,
                percentage: d.percentage || 0,
                name: d.country_name,
                code: d.code,
              });
            }
          });
        }
        this.geoJsonLayer.eachLayer((layer) => {
          const feature = layer.feature;
          const countryName = feature.properties.name;
          const countryId = feature.id;
          const countryData = dataMap.get(countryName) || dataMap.get(countryId);
          const fillColor = countryData
            ? this.getColorForValue(countryData.visitors, maxValue)
            : "#e5e5e5";
          layer.setStyle({
            fillColor: fillColor,
            weight: 0.5,
            opacity: 0.8,
            color: "#999",
            fillOpacity: 0.7,
          });
          const tooltipContent = countryData
            ? this.formatTooltipText(countryData.name, countryData.visitors, countryData.percentage)
            : `<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
                 <div style="font-weight: 500; font-size: 14px; color: #1a1a1a;">${countryName || "Unknown"}</div>
                 <div style="font-size: 13px; color: #666666;">No visitors</div>
               </div>`;
          layer.setTooltipContent(tooltipContent);
        });
        console.log("Choropleth map updated successfully");
      } catch (error) {
        console.error("Failed to update choropleth map:", error);
      }
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
          // Clear local storage
          localStorage.removeItem("kaunta_website");
          localStorage.removeItem("kaunta_dateRange");
          // Redirect to login
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
