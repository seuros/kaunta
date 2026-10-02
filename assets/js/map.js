/**
 * Kaunta Map: SVG choropleth built from the vendored world-atlas TopoJSON.
 *
 * No tiles, no WebGL: topojson-client decodes assets/data/countries-110m.json,
 * d3-geo projects it, and this file paints <path> fills from the map signal.
 * Country matching keys on ISO 3166-1 numeric codes, which is what both the
 * world-atlas feature ids and the server's `code` field carry.
 *
 * Data flows via Datastar: the Map tab's data-effect calls
 * window.kaunta.choropleth($_mapData) whenever the signal changes.
 */

(function () {
  var WIDTH = 960;
  var HEIGHT = 500;

  var featuresPromise = null;
  var rendered = null;
  var renderVersion = 0;

  function loadFeatures() {
    if (!featuresPromise) {
      featuresPromise = fetch("/assets/data/countries-110m.json")
        .then(function (response) {
          if (!response.ok) throw new Error("topology fetch failed: " + response.status);
          return response.json();
        })
        .then(function (topology) {
          return topojson.feature(topology, topology.objects.countries).features;
        });
    }
    return featuresPromise;
  }

  function colorFor(visitors, maxVisitors) {
    if (!visitors) return "var(--map-empty, #eceae5)";
    var intensity = visitors / maxVisitors;
    if (intensity < 0.2) return "#b7d3f6";
    if (intensity < 0.4) return "#6da7ec";
    if (intensity < 0.6) return "#2a78d6";
    if (intensity < 0.8) return "#1c5cab";
    return "#0d366b";
  }

  function buildSvg(container, features) {
    var projection = d3.geoNaturalEarth1().fitSize([WIDTH, HEIGHT], {
      type: "FeatureCollection",
      features: features,
    });
    var path = d3.geoPath(projection);

    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 " + WIDTH + " " + HEIGHT);
    svg.setAttribute("role", "img");
    svg.setAttribute("aria-label", "Visitors by country");
    svg.style.width = "100%";
    svg.style.height = "auto";

    var pathsById = {};
    features.forEach(function (feature) {
      var d = path(feature);
      if (!d) return;
      var el = document.createElementNS("http://www.w3.org/2000/svg", "path");
      el.setAttribute("d", d);
      el.setAttribute("stroke", "var(--map-border, #ffffff)");
      el.setAttribute("stroke-width", "0.5");
      var title = document.createElementNS("http://www.w3.org/2000/svg", "title");
      el.appendChild(title);
      svg.appendChild(el);
      pathsById[String(feature.id)] = { el: el, title: title, name: (feature.properties || {}).name || "" };
    });

    container.replaceChildren(svg);
    return { svg: svg, pathsById: pathsById };
  }

  function fill(pathsById, data) {
    var byCode = {};
    var maxVisitors = 0;
    (data || []).forEach(function (entry) {
      byCode[String(entry.code)] = entry;
      if (entry.visitors > maxVisitors) maxVisitors = entry.visitors;
    });

    Object.keys(pathsById).forEach(function (id) {
      var node = pathsById[id];
      var entry = byCode[id];
      node.el.setAttribute("fill", colorFor(entry ? entry.visitors : 0, maxVisitors || 1));
      node.title.textContent = entry
        ? node.name + " · " + entry.visitors.toLocaleString() + " visitors (" + entry.percentage.toFixed(1) + "%)"
        : node.name;
    });
  }

  window.kaunta = window.kaunta || {};
  window.kaunta.choropleth = function (data, loading, error) {
    var container = document.getElementById("choropleth-map");
    if (!container || typeof topojson === "undefined" || typeof d3 === "undefined") return;
    var version = ++renderVersion;
    container.setAttribute("aria-busy", String(!!loading));
    if (loading || error || !(data || []).some(function (entry) { return entry.visitors > 0; })) {
      container.replaceChildren(window.kaunta.emptyState(
        loading ? "Loading visitor locations…" : error ? "Visitor locations could not be loaded." : "No visitor locations in this period.",
        loading || error ? "" : "Countries will appear as visits arrive.", !!loading));
      return;
    }
    if (rendered && rendered.svg.isConnected) {
      fill(rendered.pathsById, data);
      return;
    }
    loadFeatures()
      .then(function (features) {
        if (version !== renderVersion) return;
        rendered = buildSvg(container, features);
        fill(rendered.pathsById, data);
      })
      .catch(function () {
        if (version !== renderVersion) return;
        container.replaceChildren(window.kaunta.emptyState("The map could not be loaded.", "Visitor locations are listed below."));
        featuresPromise = null;
      });
  };
})();
