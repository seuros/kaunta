/**
 * Kaunta Map Dashboard Alpine.js Component
 * Interactive visitor world map with geographic analytics
 */
// eslint-disable-next-line no-unused-vars
function mapDashboard() {
  return {
    websites: [],
    websitesLoading: true,
    websitesError: false,
    selectedWebsite: localStorage.getItem("kaunta_website") || "",
    dateRange: localStorage.getItem("kaunta_dateRange") || "1",
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
    mapInstance: null,
    geoJsonLayer: null,
    initialized: false,

    async init() {
      if (this.initialized) return;
      this.initialized = true;
      // Load filters from URL parameters
      const urlParams = new URLSearchParams(window.location.search);
      if (urlParams.get("country"))
        this.filters.country = urlParams.get("country");
      if (urlParams.get("browser"))
        this.filters.browser = urlParams.get("browser");
      if (urlParams.get("device"))
        this.filters.device = urlParams.get("device");
      if (urlParams.get("page")) this.filters.page = urlParams.get("page");

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

        // Only load data after website is determined
        if (this.selectedWebsite) {
          await this.loadAvailableFilters();
          await this.loadMapData();
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
        await this.loadAvailableFilters();
        await this.loadMapData();
      }
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
      await this.loadMapData();
    },

    async loadAvailableFilters() {
      if (!this.selectedWebsite) return;
      try {
        const countriesRes = await fetch(
          `/api/dashboard/countries/${this.selectedWebsite}?limit=100`,
        );
        if (countriesRes.ok) {
          this.availableFilters.countries = (await countriesRes.json()).data;
        }
        const browsersRes = await fetch(
          `/api/dashboard/browsers/${this.selectedWebsite}?limit=100`,
        );
        if (browsersRes.ok) {
          this.availableFilters.browsers = (await browsersRes.json()).data;
        }
        const devicesRes = await fetch(
          `/api/dashboard/devices/${this.selectedWebsite}?limit=100`,
        );
        if (devicesRes.ok) {
          this.availableFilters.devices = (await devicesRes.json()).data;
        }
        const pagesRes = await fetch(
          `/api/dashboard/pages/${this.selectedWebsite}?limit=100`,
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
        this.filters.country ||
        this.filters.browser ||
        this.filters.device ||
        this.filters.page
      );
    },

    async loadMapData() {
      if (!this.selectedWebsite) return;
      this.mapLoading = true;
      this.cleanupMap(); // Always cleanup before loading new data
      try {
        const days =
          this.dateRange === "1" ? 1 : this.dateRange === "7" ? 7 : 30;
        const params = new URLSearchParams({ days });
        if (this.filters.country)
          params.append("country", this.filters.country);
        if (this.filters.browser)
          params.append("browser", this.filters.browser);
        if (this.filters.device) params.append("device", this.filters.device);
        if (this.filters.page) params.append("page", this.filters.page);
        const response = await fetch(
          `/api/dashboard/map/${this.selectedWebsite}?${params}`,
        );
        if (response.ok) {
          this.mapData = await response.json();
          // Wait for DOM and then initialize with retry
          await this.waitForContainerAndInitialize();
        } else {
          console.error("Failed to load map data");
        }
      } catch (error) {
        console.error("Map data error:", error);
      } finally {
        this.mapLoading = false;
      }
    },

    async waitForContainerAndInitialize(retries = 3) {
      for (let i = 0; i < retries; i++) {
        const container = document.getElementById("choropleth-map");
        if (container) {
          await this.initializeChoropleth();
          return;
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      console.warn("Map container not found after retries");
    },

    fixAntimeridianRing(ring) {
      const threshold = 180; // degrees
      for (let i = 1; i < ring.length; i++) {
        const prevLon = ring[i - 1][0];
        const currLon = ring[i][0];
        const diff = Math.abs(currLon - prevLon);

        if (diff > threshold) {
          if (currLon > prevLon) {
            // Eastern to western hemisphere
            ring[i][0] = currLon - 360;
          } else {
            // Western to eastern hemisphere
            ring[i][0] = currLon + 360;
          }
        }
      }
      return ring;
    },

    cleanupMap() {
      if (this.mapInstance) {
        try {
          this.mapInstance.remove();
        } catch (e) {
          console.warn("Error removing map:", e);
        }
        this.mapInstance = null;
        this.geoJsonLayer = null;
      }
      const container = document.getElementById("choropleth-map");
      if (container) {
        container.innerHTML = "";
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
    },

    async initializeChoropleth() {
      try {
        const container = document.getElementById("choropleth-map");
        if (!container) {
          console.error("Choropleth map container not found");
          return;
        }

        // Clean up any existing map
        this.cleanupMap();

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
        let features = topojson.feature(
          world,
          world.objects.countries,
        ).features;

        // Fix antimeridian crossing issues
        features = features.map((feature) => {
          if (feature.geometry && feature.geometry.coordinates) {
            // Handle MultiPolygon and Polygon geometries
            if (feature.geometry.type === "MultiPolygon") {
              feature.geometry.coordinates = feature.geometry.coordinates.map(
                (polygon) =>
                  polygon.map((ring) => this.fixAntimeridianRing(ring)),
              );
            } else if (feature.geometry.type === "Polygon") {
              feature.geometry.coordinates = feature.geometry.coordinates.map(
                (ring) => this.fixAntimeridianRing(ring),
              );
            }
          }
          return feature;
        });
        const dataMap = new Map();
        let maxValue = 0;
        if (
          this.mapData &&
          this.mapData.data &&
          Array.isArray(this.mapData.data)
        ) {
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
          const countryData =
            dataMap.get(countryName) || dataMap.get(countryId);
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
          const countryData =
            dataMap.get(countryName) || dataMap.get(countryId);
          const tooltipContent = countryData
            ? this.formatTooltipText(
                countryData.name,
                countryData.visitors,
                countryData.percentage,
              )
            : `<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
                   <div style="font-weight: 500; font-size: 14px; color: #1a1a1a;">${
                     countryName || "Unknown"
                   }</div>
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
        if (
          this.mapData &&
          this.mapData.data &&
          Array.isArray(this.mapData.data)
        ) {
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
          const countryData =
            dataMap.get(countryName) || dataMap.get(countryId);
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
            ? this.formatTooltipText(
                countryData.name,
                countryData.visitors,
                countryData.percentage,
              )
            : `<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
                   <div style="font-weight: 500; font-size: 14px; color: #1a1a1a;">${
                     countryName || "Unknown"
                   }</div>
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
