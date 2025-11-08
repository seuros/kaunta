/**
 * Kaunta Analytics Tracker
 * Privacy-first, lightweight analytics tracker combining best features from Umami & Plausible
 *
 * Features:
 * - Auto-track pageviews (including SPAs)
 * - Outbound link tracking
 * - Custom event tracking
 * - Scroll depth tracking
 * - Engagement time tracking
 * - Respects Do Not Track
 * - No cookies, no localStorage (privacy-first)
 * - <3KB minified
 *
 * @version 1.0.0
 */

(function(window) {
  'use strict';

  // Early exit checks - use window.document directly to avoid minification issues
  if (!window || !window.document) return;

  var {
    screen: { width, height },
    navigator: { language, doNotTrack: ndnt, msDoNotTrack: msdnt },
    location,
    document,
    history,
    doNotTrack
  } = window;

  var { currentScript, referrer } = document;

  // Fallback: if currentScript is null (defer/async loading), find the script tag
  if (!currentScript) {
    var scripts = document.querySelectorAll('script[data-website-id]');
    if (scripts.length > 0) {
      currentScript = scripts[scripts.length - 1]; // Use the last one
    } else {
      // Try to find script with k.js, kaunta.js, or script.js
      var allScripts = document.querySelectorAll('script[src]');
      for (var i = 0; i < allScripts.length; i++) {
        var src = allScripts[i].src || '';
        if (src.indexOf('k.js') > -1 || src.indexOf('kaunta.js') > -1 || src.indexOf('script.js') > -1) {
          currentScript = allScripts[i];
          break;
        }
      }
    }
  }

  if (!currentScript) return;

  // ============================================================================
  // CONFIGURATION (from data attributes)
  // ============================================================================

  var dataset = currentScript.dataset;

  var websiteId = dataset.websiteId;
  var apiUrl = dataset.apiUrl || currentScript.src.split('/').slice(0, -1).join('/');
  var autoTrack = dataset.autoTrack !== 'false';
  var trackOutbound = dataset.trackOutbound !== 'false';
  var respectDnt = dataset.respectDnt !== 'false';
  var excludeHash = dataset.excludeHash === 'true';
  var domain = dataset.domains || '';
  var domains = domain.split(',').map(function(n) {
    return n.trim().toLowerCase().replace(/:\d+$/, '');
  });

  var endpoint = apiUrl.replace(/\/$/, '') + '/api/send';
  var screen = width + 'x' + height;
  var { hostname, origin } = location;

  // Static payload fields that don't change per event
  var staticPayload = Object.freeze({
    website: websiteId,
    hostname: hostname,
    screen: screen,
    language: language
  });

  // ============================================================================
  // ENGAGEMENT & SCROLL TRACKING (from Plausible)
  // ============================================================================

  var debug = dataset.debug === 'true';

  function logDebug() {
    if (!debug || !window.console) return;
    var args = Array.prototype.slice.call(arguments);
    args.unshift('[Kaunta]');
    try {
      console.debug.apply(console, args);
    } catch (err) {
      try {
        console.log.apply(console, args);
      } catch (_) {
        // ignore
      }
    }
  }

  logDebug('Tracker initialized', { apiUrl: apiUrl, websiteId: websiteId });

  var engagementListening = false;
  var scrollScheduled = false;
  var heightObserver = null;
  var engagementAbort = null;
  var currentPageUrl = location.href;
  var maxScrollDepthPx = 0;
  var currentDocHeight = 0;
  var engagementStartTime = 0;
  var totalEngagementTime = 0;
  var engagementIgnored = false;

  function getDocHeight() {
    var body = document.body || {};
    var el = document.documentElement || {};
    return Math.max(
      body.scrollHeight || 0,
      body.offsetHeight || 0,
      body.clientHeight || 0,
      el.scrollHeight || 0,
      el.offsetHeight || 0,
      el.clientHeight || 0
    );
  }

  function getCurrentScrollDepthPx() {
    var body = document.body || {};
    var el = document.documentElement || {};
    var viewportHeight = window.innerHeight || el.clientHeight || 0;
    var scrollTop = window.scrollY || el.scrollTop || body.scrollTop || 0;

    return currentDocHeight <= viewportHeight
      ? currentDocHeight
      : scrollTop + viewportHeight;
  }

  function getEngagementTime() {
    if (engagementStartTime) {
      return totalEngagementTime + (Date.now() - engagementStartTime);
    }
    return totalEngagementTime;
  }

  function updateScrollDepth() {
    currentDocHeight = getDocHeight();
    var currentScrollDepth = getCurrentScrollDepthPx();

    if (currentScrollDepth > maxScrollDepthPx) {
      maxScrollDepthPx = currentScrollDepth;
    }
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'visible' && document.hasFocus() && engagementStartTime === 0) {
      engagementStartTime = Date.now();
    } else if (document.visibilityState === 'hidden' || !document.hasFocus()) {
      // Save engagement time
      totalEngagementTime = getEngagementTime();
      engagementStartTime = 0;
    }
  }

  function initEngagementTracking() {
    if (!engagementListening) {
      currentDocHeight = getDocHeight();
      maxScrollDepthPx = getCurrentScrollDepthPx();

      // Create AbortController for cleanup
      engagementAbort = window.AbortController ? new AbortController() : null;
      var signal = engagementAbort ? { signal: engagementAbort.signal } : {};

      // rAF-batched scroll tracking to prevent layout thrashing
      document.addEventListener('scroll', function() {
        if (scrollScheduled) return;
        scrollScheduled = true;
        requestAnimationFrame(function() {
          scrollScheduled = false;
          updateScrollDepth();
        });
      }, Object.assign({ passive: true }, signal));

      document.addEventListener('visibilitychange', onVisibilityChange, Object.assign({ passive: true }, signal));
      window.addEventListener('blur', onVisibilityChange, Object.assign({ passive: true }, signal));
      window.addEventListener('focus', onVisibilityChange, Object.assign({ passive: true }, signal));

      // Use ResizeObserver to track document height changes efficiently
      if (window.ResizeObserver) {
        heightObserver = new ResizeObserver(function() {
          currentDocHeight = getDocHeight();
        });
        heightObserver.observe(document.documentElement);
        if (document.body) {
          heightObserver.observe(document.body);
        }
      } else {
        // Fallback for older browsers
        window.addEventListener('load', function() {
          currentDocHeight = getDocHeight();
          var count = 0;
          var interval = setInterval(function() {
            currentDocHeight = getDocHeight();
            if (++count === 15) clearInterval(interval);
          }, 200);
        });
      }

      engagementListening = true;
    }
  }

  // ============================================================================
  // HELPER FUNCTIONS (from Umami)
  // ============================================================================

  function hasDoNotTrack() {
    var dnt = doNotTrack || ndnt || msdnt;
    return dnt === 1 || dnt === '1' || dnt === 'yes';
  }

  function isTrackingDisabled() {
    return !websiteId ||
      (domain && !domains.includes(hostname)) ||
      (respectDnt && hasDoNotTrack());
  }

  function normalize(url) {
    if (!url) return url;
    try {
      var u = new URL(url, location.href);
      if (excludeHash) u.hash = '';
      return u.toString();
    } catch (e) {
      return url;
    }
  }

  function getBasePayload(includeEngagement) {
    var payload = Object.assign({}, staticPayload, {
      url: currentPageUrl,
      title: document.title,
      referrer: currentRef
    });

    // Only include engagement metrics for pageviews to reduce payload size
    if (includeEngagement) {
      var scrollDepthPercent = currentDocHeight > 0
        ? Math.round((maxScrollDepthPx / currentDocHeight) * 100)
        : 0;
      var engagementTimeMs = Math.round(getEngagementTime());

      payload.scroll_depth = scrollDepthPercent;
      payload.engagement_time = engagementTimeMs;
    }

    return payload;
  }

  // ============================================================================
  // NETWORK REQUEST (from Plausible - minimal, modern)
  // ============================================================================

  function send(payload, type) {
    if (isTrackingDisabled()) {
      logDebug('Tracking disabled: SKIP', type, payload);
      return;
    }

    type = type || 'event';

    logDebug('Sending', type, payload);

    var body = JSON.stringify({ type: type, payload: payload });

    // Silent fail - no console spam unless debug
    try {
      // Use sendBeacon for better reliability when page is hidden/unloading
      if (navigator.sendBeacon && document.visibilityState === 'hidden') {
        navigator.sendBeacon(endpoint, body);
      } else if (window.fetch) {
        fetch(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: body,
          keepalive: true,
          credentials: 'omit'
        }).catch(function(err) {
          if (debug) logDebug('Fetch error', err);
        });
      }
    } catch (e) {
      if (debug) logDebug('Send exception', e);
    }
  }

  // ============================================================================
  // TRACKING FUNCTIONS
  // ============================================================================

  function trackPageview() {
    // Include engagement metrics for pageviews
    var payload = getBasePayload(true);

    // Reset engagement tracking for new page
    maxScrollDepthPx = getCurrentScrollDepthPx();
    totalEngagementTime = 0;
    engagementStartTime = Date.now();
    engagementIgnored = false;

    send(payload, 'event');
  }

  function track(eventName, properties) {
    if (typeof eventName !== 'string') return;

    // Don't include engagement metrics for custom events
    var payload = getBasePayload(false);
    payload.name = eventName;

    if (properties && typeof properties === 'object') {
      payload.props = properties;
    }

    send(payload, 'event');
  }

  // ============================================================================
  // AUTO-TRACKING: SPA NAVIGATION (from both Umami & Plausible)
  // ============================================================================

  var lastPath = location.pathname;
  var pendingPageview = null;

  function onNavigation() {
    var newPath = location.pathname;
    var newUrl = normalize(location.href);

    if (lastPath === newPath && newUrl === currentPageUrl) return;

    lastPath = newPath;
    currentRef = currentPageUrl;
    currentPageUrl = newUrl;

    if (currentPageUrl !== currentRef) {
      // Debounce to prevent duplicate pageviews on rapid navigation
      clearTimeout(pendingPageview);
      pendingPageview = setTimeout(trackPageview, 150);
    }
  }

  function hookHistory() {
    var hook = function(obj, method, callback) {
      var orig = obj[method];
      if (typeof orig !== 'function') return;
      obj[method] = function() {
        var result = orig.apply(this, arguments);
        callback.apply(null, arguments);
        return result;
      };
    };

    hook(history, 'pushState', onNavigation);
    hook(history, 'replaceState', onNavigation);
    window.addEventListener('popstate', onNavigation);
  }

  // ============================================================================
  // AUTO-TRACKING: OUTBOUND LINKS (from Plausible)
  // ============================================================================

  function isOutboundLink(link) {
    return link &&
      typeof link.href === 'string' &&
      link.host &&
      link.host !== location.host;
  }

  function getLinkElement(el) {
    while (el && (typeof el.tagName === 'undefined' || el.tagName.toLowerCase() !== 'a' || !el.href)) {
      el = el.parentNode;
    }
    return el;
  }

  function shouldInterceptNav(event, link) {
    if (event.defaultPrevented) return false;

    var target = link.target;
    if (target && typeof target === 'string' && !target.match(/^_(self|parent|top)$/i)) {
      return false;
    }

    if (event.ctrlKey || event.metaKey || event.shiftKey || event.type !== 'click') {
      return false;
    }

    return true;
  }

  function onLinkClick(event) {
    var link = getLinkElement(event.target);

    if (trackOutbound && isOutboundLink(link)) {
      var followed = false;

      var followLink = function() {
        if (!followed) {
          followed = true;
          window.location = link.href;
        }
      };

      // Track the outbound click
      track('Outbound Link: Click', { url: normalize(link.href) });

      if (shouldInterceptNav(event, link)) {
        event.preventDefault();
        setTimeout(followLink, 500); // Give analytics 500ms to send
      }
    }
  }

  // ============================================================================
  // INITIALIZATION
  // ============================================================================

  currentPageUrl = normalize(location.href);
  var currentRef = normalize((referrer || '').startsWith(origin) ? '' : referrer);
  var initialized = false;

  function init() {
    if (initialized || isTrackingDisabled()) return;

    initialized = true;

    // Initialize tracking systems
    initEngagementTracking();
    hookHistory();

    // Track initial pageview
    trackPageview();

    // Setup click handlers for outbound links
    if (trackOutbound) {
      document.addEventListener('click', onLinkClick, true);
    }
  }

  // ============================================================================
  // PUBLIC API
  // ============================================================================

  function destroy() {
    // Abort all event listeners
    if (engagementAbort) {
      engagementAbort.abort();
    }

    // Disconnect ResizeObserver
    if (heightObserver) {
      heightObserver.disconnect();
    }

    // Clear pending pageview
    clearTimeout(pendingPageview);

    // Reset state
    initialized = false;
    engagementListening = false;

    logDebug('Tracker destroyed');
  }

  if (!window.kaunta) {
    window.kaunta = {
      track: track,
      trackPageview: trackPageview,
      destroy: destroy
    };
  }

  // ============================================================================
  // AUTO-START
  // ============================================================================

  if (autoTrack && !isTrackingDisabled()) {
    if (document.readyState === 'complete') {
      init();
    } else {
      window.addEventListener('load', init);
    }
  }

})(window);
