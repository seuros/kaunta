/**
 * Kaunta Map Dashboard - Datastar Edition
 * Leaflet map initialization and helper functions
 * Data flows via SSE signals from the server
 */

// Map instance references
let mapInstance = null;
let geoJsonLayer = null;

// Color scale for choropleth
function getColorForValue(value, maxValue) {
  if (!value || value === 0) return "#e5e5e5";
  const intensity = value / maxValue;
  if (intensity < 0.2) return "#deebf7";
  if (intensity < 0.4) return "#9ecae1";
  if (intensity < 0.6) return "#6baed6";
  if (intensity < 0.8) return "#3182bd";
  return "#08519c";
}

// Format tooltip content
function formatTooltipText(countryName, visitors, percentage) {
  return `
    <div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
      <div style="font-weight: 500; font-size: 14px; margin-bottom: 4px; color: #1a1a1a;">${countryName}</div>
      <div style="font-size: 13px; color: #666666;">
        ${visitors.toLocaleString()} visitors (${percentage.toFixed(1)}%)
      </div>
    </div>
  `;
}

// Fix antimeridian crossing issues in polygon rings
function fixAntimeridianRing(ring) {
  const threshold = 180;
  for (let i = 1; i < ring.length; i++) {
    const prevLon = ring[i - 1][0];
    const currLon = ring[i][0];
    const diff = Math.abs(currLon - prevLon);

    if (diff > threshold) {
      if (currLon > prevLon) {
        ring[i][0] = currLon - 360;
      } else {
        ring[i][0] = currLon + 360;
      }
    }
  }
  return ring;
}

// Cleanup existing map instance
window.cleanupMap = function() {
  if (mapInstance) {
    try {
      mapInstance.remove();
    } catch (e) {
      console.warn("Error removing map:", e);
    }
    mapInstance = null;
    geoJsonLayer = null;
  }
  const container = document.getElementById("choropleth-map");
  if (container) {
    container.innerHTML = "";
  }
};

// Initialize the choropleth map - called via SSE ExecuteScript
window.initChoroplethMap = async function(mapData) {
  try {
    const container = document.getElementById("choropleth-map");
    if (!container) {
      console.error("Choropleth map container not found");
      return;
    }

    // Clean up any existing map
    window.cleanupMap();

    // Create map instance
    const map = L.map("choropleth-map", {
      zoomControl: true,
      attributionControl: true,
      scrollWheelZoom: true,
      doubleClickZoom: true,
      touchZoom: true,
    }).setView([20, 0], 2);

    // Add tile layer
    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
      attribution:
        '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
      maxZoom: 18,
      minZoom: 1,
    }).addTo(map);

    // Load TopoJSON world data
    const response = await fetch("/assets/data/countries-110m.json");
    if (!response.ok) {
      throw new Error(`Failed to load TopoJSON: ${response.statusText}`);
    }
    const world = await response.json();
    let features = topojson.feature(world, world.objects.countries).features;

    // Fix antimeridian crossing issues
    features = features.map((feature) => {
      if (feature.geometry && feature.geometry.coordinates) {
        if (feature.geometry.type === "MultiPolygon") {
          feature.geometry.coordinates = feature.geometry.coordinates.map(
            (polygon) => polygon.map((ring) => fixAntimeridianRing(ring))
          );
        } else if (feature.geometry.type === "Polygon") {
          feature.geometry.coordinates = feature.geometry.coordinates.map(
            (ring) => fixAntimeridianRing(ring)
          );
        }
      }
      return feature;
    });

    // Build data map from server data
    const dataMap = new Map();
    let maxValue = 0;
    if (mapData && mapData.data && Array.isArray(mapData.data)) {
      maxValue = Math.max(...mapData.data.map((d) => d.visitors || 0));
      mapData.data.forEach((d) => {
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

    // Style function for features
    const getStyle = (feature) => {
      const countryName = feature.properties.name;
      const countryId = feature.id;
      const countryData = dataMap.get(countryName) || dataMap.get(countryId);
      const fillColor = countryData
        ? getColorForValue(countryData.visitors, maxValue)
        : "#e5e5e5";
      return {
        fillColor: fillColor,
        weight: 0.5,
        opacity: 0.8,
        color: "#999",
        fillOpacity: 0.7,
      };
    };

    // Feature interaction handler
    const onEachFeature = (feature, layer) => {
      const countryName = feature.properties.name;
      const countryId = feature.id;
      const countryData = dataMap.get(countryName) || dataMap.get(countryId);

      const tooltipContent = countryData
        ? formatTooltipText(
            countryData.name,
            countryData.visitors,
            countryData.percentage
          )
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
          // Apply country filter via Datastar
          const filterUrl = `/api/dashboard/map-filter-ds?country=${encodeURIComponent(countryData.name)}`;
          // Trigger SSE request to apply filter
          window.dispatchEvent(new CustomEvent('datastar-get', { detail: { url: filterUrl } }));
        });
        layer.on("mouseover", function () {
          container.style.cursor = "pointer";
        });
        layer.on("mouseout", function () {
          container.style.cursor = "";
        });
      }
    };

    // Add GeoJSON layer to map
    const layer = L.geoJSON(features, {
      style: getStyle,
      onEachFeature: onEachFeature,
    }).addTo(map);

    // Store references
    mapInstance = map;
    geoJsonLayer = layer;

    // Add custom tooltip styles if not present
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

    console.log("Choropleth map initialized successfully");
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
};

// Update existing map with new data - called via SSE ExecuteScript
window.updateChoroplethMap = function(mapData) {
  try {
    if (!mapInstance || !geoJsonLayer) {
      console.warn("Map not initialized, cannot update");
      return;
    }

    const dataMap = new Map();
    let maxValue = 0;
    if (mapData && mapData.data && Array.isArray(mapData.data)) {
      maxValue = Math.max(...mapData.data.map((d) => d.visitors || 0));
      mapData.data.forEach((d) => {
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

    geoJsonLayer.eachLayer((layer) => {
      const feature = layer.feature;
      const countryName = feature.properties.name;
      const countryId = feature.id;
      const countryData = dataMap.get(countryName) || dataMap.get(countryId);

      const fillColor = countryData
        ? getColorForValue(countryData.visitors, maxValue)
        : "#e5e5e5";

      layer.setStyle({
        fillColor: fillColor,
        weight: 0.5,
        opacity: 0.8,
        color: "#999",
        fillOpacity: 0.7,
      });

      const tooltipContent = countryData
        ? formatTooltipText(
            countryData.name,
            countryData.visitors,
            countryData.percentage
          )
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
};

// Cleanup on page unload
window.addEventListener("beforeunload", function() {
  window.cleanupMap();
});
