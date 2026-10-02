/**
 * Kaunta analytics tracker.
 *
 * Records pageviews (including SPA navigation), outbound clicks, file
 * downloads, custom events, scroll depth, and engagement time, and carries
 * UTM parameters for the session.
 *
 * It sets no cookies and sends no identifiers it did not receive. The only
 * thing it stores is the visitor's own choices: UTM parameters in
 * sessionStorage for the length of the visit, and the `kaunta_ignore`
 * opt-out flag in localStorage. Do Not Track is honored unless the site
 * turns that off.
 */

(function(window) {
  'use strict';

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

  if (!currentScript) {
    var scripts = document.querySelectorAll('script[data-website-id]');
    if (scripts.length > 0) {
      currentScript = scripts[scripts.length - 1];
    } else {
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

  var dataset = currentScript.dataset;

  var websiteId = dataset.websiteId;
  var apiUrl = dataset.apiUrl || currentScript.src.split('/').slice(0, -1).join('/');
  var autoTrack = dataset.autoTrack !== 'false';
  var trackOutbound = dataset.trackOutbound !== 'false';
  var trackDownloads = dataset.trackDownloads !== 'false';
  var downloadExtensions = (dataset.downloadExtensions || 'pdf,xlsx,docx,txt,rtf,csv,exe,key,pps,ppt,pptx,7z,pkg,rar,gz,zip,avi,mov,mp4,mpeg,wmv,midi,mp3,wav,ogg,dmg')
    .split(',').map(function(ext) { return ext.trim().toLowerCase(); });
  var respectDnt = dataset.respectDnt !== 'false';
  var excludeHash = dataset.excludeHash === 'true';
  var domain = dataset.domains || '';
  var domains = domain.split(',').map(function(n) {
    return n.trim().toLowerCase().replace(/:\d+$/, '');
  });

  var endpoint = apiUrl.replace(/\/$/, '') + '/api/send';
  var screen = width + 'x' + height;
  var { hostname, origin } = location;

  var UTM_PARAMS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content'];
  var UTM_STORAGE_KEY = 'kaunta_utm';

  function getUtmParams() {
    var stored = null;
    try {
      var storedStr = sessionStorage.getItem(UTM_STORAGE_KEY);
      if (storedStr) {
        stored = JSON.parse(storedStr);
      }
    } catch (e) {}

    var searchParams = new URLSearchParams(location.search);
    var currentUtm = {};
    var hasNewUtm = false;

    UTM_PARAMS.forEach(function(param) {
      var value = searchParams.get(param);
      if (value) {
        currentUtm[param] = value;
        hasNewUtm = true;
      }
    });

    if (hasNewUtm) {
      try {
        sessionStorage.setItem(UTM_STORAGE_KEY, JSON.stringify(currentUtm));
      } catch (e) {}
      return currentUtm;
    }

    return stored || {};
  }

  var utmParams = getUtmParams();

  var staticPayload = Object.freeze({
    website: websiteId,
    hostname: hostname,
    screen: screen,
    language: language
  });

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
      } catch (_) {}
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

  var lastFlushedEngagementMs = 0;

  function flushEngagement() {
    var engagementMs = Math.round(getEngagementTime());
    var scrollPercent = currentDocHeight > 0
      ? Math.round((maxScrollDepthPx / currentDocHeight) * 100)
      : 0;
    if (engagementMs < 1000 && scrollPercent < 10) return;
    if (engagementMs <= lastFlushedEngagementMs) return;
    lastFlushedEngagementMs = engagementMs;
    send(getBasePayload(true), 'engagement');
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'visible' && document.hasFocus() && engagementStartTime === 0) {
      engagementStartTime = Date.now();
    } else if (document.visibilityState === 'hidden' || !document.hasFocus()) {
      totalEngagementTime = getEngagementTime();
      engagementStartTime = 0;
      if (document.visibilityState === 'hidden') {
        flushEngagement();
      }
    }
  }

  function initEngagementTracking() {
    if (!engagementListening) {
      currentDocHeight = getDocHeight();
      maxScrollDepthPx = getCurrentScrollDepthPx();

      engagementAbort = window.AbortController ? new AbortController() : null;
      var signal = engagementAbort ? { signal: engagementAbort.signal } : {};

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
      window.addEventListener('pagehide', flushEngagement, Object.assign({ passive: true }, signal));

      if (window.ResizeObserver) {
        heightObserver = new ResizeObserver(function() {
          currentDocHeight = getDocHeight();
        });
        heightObserver.observe(document.documentElement);
        if (document.body) {
          heightObserver.observe(document.body);
        }
      } else {
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

  function hasDoNotTrack() {
    var dnt = doNotTrack || ndnt || msdnt;
    return dnt === 1 || dnt === '1' || dnt === 'yes';
  }

  var IGNORE_KEY = 'kaunta_ignore';

  function readIgnoreFlag() {
    var requested = new URLSearchParams(location.search).get('kaunta_ignore');
    try {
      if (requested === 'true') {
        localStorage.setItem(IGNORE_KEY, 'true');
        return true;
      }
      if (requested === 'false') {
        localStorage.removeItem(IGNORE_KEY);
        return false;
      }
      return localStorage.getItem(IGNORE_KEY) === 'true';
    } catch (e) {
      return requested === 'true';
    }
  }

  var ignored = readIgnoreFlag();

  function isTrackingDisabled() {
    return !websiteId ||
      ignored ||
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

    if (includeEngagement) {
      var scrollDepthPercent = currentDocHeight > 0
        ? Math.round((maxScrollDepthPx / currentDocHeight) * 100)
        : 0;
      var engagementTimeMs = Math.round(getEngagementTime());

      payload.scroll_depth = scrollDepthPercent;
      payload.engagement_time = engagementTimeMs;
    }

    if (utmParams.utm_source) payload.utm_source = utmParams.utm_source;
    if (utmParams.utm_medium) payload.utm_medium = utmParams.utm_medium;
    if (utmParams.utm_campaign) payload.utm_campaign = utmParams.utm_campaign;
    if (utmParams.utm_term) payload.utm_term = utmParams.utm_term;
    if (utmParams.utm_content) payload.utm_content = utmParams.utm_content;

    return payload;
  }

  function send(payload, type) {
    if (isTrackingDisabled()) {
      logDebug('Tracking disabled: SKIP', type, payload);
      return;
    }

    type = type || 'event';

    logDebug('Sending', type, payload);

    var body = JSON.stringify({ type: type, payload: payload });

    try {
      var isSameOrigin = endpoint.indexOf(origin) === 0;
      var credentialsMode = isSameOrigin ? 'same-origin' : 'omit';

      if (navigator.sendBeacon && document.visibilityState === 'hidden') {
        navigator.sendBeacon(endpoint, body);
      } else if (window.fetch) {
        fetch(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: body,
          keepalive: true,
          credentials: credentialsMode
        }).catch(function(err) {
          if (debug) logDebug('Fetch error', err);
        });
      }
    } catch (e) {
      if (debug) logDebug('Send exception', e);
    }
  }

  function trackPageview() {
    var payload = getBasePayload(true);

    maxScrollDepthPx = getCurrentScrollDepthPx();
    totalEngagementTime = 0;
    engagementStartTime = Date.now();
    engagementIgnored = false;
    lastFlushedEngagementMs = 0;

    send(payload, 'event');
  }

  function track(eventName, properties) {
    if (typeof eventName !== 'string') return;

    var payload = getBasePayload(false);
    payload.name = eventName;

    if (properties && typeof properties === 'object') {
      payload.props = properties;
    }

    send(payload, 'event');
  }

  var lastPath = location.pathname;
  var pendingPageview = null;

  function onNavigation() {
    var newPath = location.pathname;
    var newUrl = normalize(location.href);

    if (lastPath === newPath && newUrl === currentPageUrl) return;

    lastPath = newPath;
    currentRef = currentPageUrl;
    currentPageUrl = newUrl;

    utmParams = getUtmParams();

    if (currentPageUrl !== currentRef) {
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

  function isOutboundLink(link) {
    return link &&
      typeof link.href === 'string' &&
      link.host &&
      link.host !== location.host;
  }

  function isDownloadLink(link) {
    if (!link || !link.pathname) return false;
    var path = link.pathname.toLowerCase();
    var dot = path.lastIndexOf('.');
    return dot !== -1 && downloadExtensions.indexOf(path.slice(dot + 1)) !== -1;
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
    var download = trackDownloads && isDownloadLink(link);

    if (download || (trackOutbound && isOutboundLink(link))) {
      var followed = false;

      var followLink = function() {
        if (!followed) {
          followed = true;
          window.location = link.href;
        }
      };

      track(download ? 'File Download' : 'Outbound Link: Click',
        { url: normalize(link.href) });

      if (shouldInterceptNav(event, link)) {
        event.preventDefault();
        setTimeout(followLink, 500);
      }
    }
  }

  currentPageUrl = normalize(location.href);
  var currentRef = normalize((referrer || '').startsWith(origin) ? '' : referrer);
  var initialized = false;

  function init() {
    if (initialized || isTrackingDisabled()) return;

    initialized = true;

    initEngagementTracking();
    hookHistory();

    trackPageview();

    if (trackOutbound || trackDownloads) {
      document.addEventListener('click', onLinkClick, true);
    }
  }

  function destroy() {
    if (engagementAbort) {
      engagementAbort.abort();
    }

    if (heightObserver) {
      heightObserver.disconnect();
    }

    clearTimeout(pendingPageview);

    initialized = false;
    engagementListening = false;

    logDebug('Tracker destroyed');
  }

  if (!window.kaunta || typeof window.kaunta.track !== 'function') {
    window.kaunta = Object.assign(window.kaunta || {}, {
      track: track,
      trackPageview: trackPageview,
      destroy: destroy
    });
  }

  if (autoTrack && !isTrackingDisabled()) {
    if (document.readyState === 'complete') {
      init();
    } else {
      window.addEventListener('load', init);
    }
  }

})(window);
