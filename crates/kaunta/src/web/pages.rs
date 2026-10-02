use maud::{DOCTYPE, Markup, html};

/// Assets are served with a year-long immutable Cache-Control (and cached by
/// Cloudflare accordingly), so every asset URL carries the crate version as a
/// cache-buster: a release changes the URL, which changes the CDN cache key.
fn asset(path: &str) -> String {
    format!("{path}?v={}", env!("CARGO_PKG_VERSION"))
}

fn head(title: &str) -> Markup {
    html! {
        head {
            meta charset="utf-8";
            meta name="viewport" content="width=device-width, initial-scale=1";
            title { (title) }
            link rel="stylesheet" href=(asset("/assets/global.css"));
            link rel="stylesheet" href=(asset("/assets/portal.css"));
            script src=(asset("/assets/js/portal.js")) defer {}
            script type="module" src=(asset("/assets/vendor/vendor.js")) {}
        }
    }
}

pub fn index(version: &str) -> Markup {
    html! {
        (DOCTYPE)
        html lang="en" {
            (head("Kaunta - Analytics without bloat"))
            body {
                main {
                    h1 { "Kaunta" }
                    p { "Analytics without bloat." }
                    a href="/login" { "Open dashboard" }
                    small { "Rust port " (version) }
                }
            }
        }
    }
}

pub fn login(version: &str) -> Markup {
    html! {
        (DOCTYPE)
        html lang="en" {
            (head("Login - Kaunta"))
            body {
                main {
                    h1 { "Login" }
                    form id="login-form" method="post" action="/api/auth/login" {
                        label for="username" { "Username" }
                        input id="username" name="username" autocomplete="username" required;
                        label for="password" { "Password" }
                        input id="password" name="password" type="password"
                            autocomplete="current-password" required;
                        button type="submit" { "Sign in" }
                        p id="login-error" role="alert" {}
                    }
                    noscript { "JavaScript is required to sign in." }
                    small { "Kaunta " (version) }
                }
            }
        }
    }
}

/// Re-arm the loading skeletons when the user changes website or period.
/// Written from event handlers only: a `$xLoading = true` inside a fetch
/// `data-effect` makes the effect depend on the signal the server patches
/// back to false, which loops the fetch forever.
const RELOADING: &str =
    "$statsLoading = true; $chartLoading = true; $breakdownLoading = true; $mapLoading = true";
const CLEAR_FILTERS: &str =
    "$filterCountry = ''; $filterBrowser = ''; $filterDevice = ''; $filterPage = ''";
const FILTERS: &str =
    "{country: $filterCountry, browser: $filterBrowser, device: $filterDevice, page: $filterPage}";

pub fn dashboard(title: &str, version: &str) -> Markup {
    let init = match title {
        "Websites" => "@get('/api/dashboard/websites-init')",
        _ => "@get('/api/dashboard/campaigns-init' + window.location.search)",
    };
    let signals = serde_json::json!({
        "_websites": [], "selectedWebsite": "", "days": "7",
        "filterCountry": "", "filterBrowser": "", "filterDevice": "", "filterPage": "",
        "websitesLoading": true, "websitesError": false,
        "statsError": false, "chartError": false, "breakdownError": false,
        "statsLoading": true, "chartLoading": true, "breakdownLoading": true, "mapLoading": true,
        "stats": {"current_visitors": 0, "today_pageviews": 0, "today_visitors": 0, "today_bounce_rate": "0%"},
        "overview": {"visitors": 0, "pageviews": 0, "bounce_rate": "0%",
                     "prev_visitors": 0, "prev_pageviews": 0, "prev_bounce_rate": "0%", "period_days": 7},
        "_timeseries": [], "_breakdown": {"items": [], "total": 0}, "breakdownType": "page",
        "breakdownPage": 1, "_mapData": [], "mapTotalVisitors": 0, "mapError": false,
        "_goals": [], "goalError": false, "goalId": "", "goalForm": {"name": "", "type": "page_view", "value": ""},
        "submitting": false, "newWebsite": {"domain": "", "name": ""}, "createError": "",
        "creating": false, "websitesReload": false, "toast": {"show": false, "message": ""}
    }).to_string();
    let signals = format!(
        "Object.assign({signals}, {{selectedWebsite: localStorage.getItem('kaunta.website') || '', \
         days: localStorage.getItem('kaunta.days') || '7'}})"
    );
    html! {
        (DOCTYPE)
        html lang="en" {
            (head(&format!("{title} - Kaunta")))
            body {
                header {
                    nav {
                        a href="/dashboard" { "Dashboard" }
                        a href="/dashboard/map" { "Map" }
                        a href="/dashboard/campaigns" { "Campaigns" }
                        a href="/dashboard/websites" { "Websites" }
                        a href="/dashboard/goals" { "Goals" }
                        button type="button" data-on:click="@post('/api/auth/logout', {headers: window.kaunta.headers()})" { "Sign out" }
                    }
                }
                main data-signals=(signals) data-init=(init) {
                    h1 { (title) }
                    p id="request-error" role="alert" {}
                    p class="shell-state is-loading" role="status" data-show="$websitesLoading" { "Loading websites…" }
                    p data-show="$websitesError" data-text="$websitesError" role="alert" {}
                    p class="shell-state" data-show="!$websitesLoading && !$websitesError && !$_websites.length" {
                        "Your analytics starts with a website. " a href="/dashboard/websites" { "Add a website" }
                    }
                    p data-show="$toast.show" data-text="$toast.message" role="status" {}
                    @if title != "Websites" {
                        div class="portal-controls" {
                            label for="website-select" { "Website" }
                            select id="website-select" data-bind="selectedWebsite"
                                data-on:change=(format!("$goalId = ''; $goalForm = {{name: '', type: 'page_view', value: ''}}; $breakdownPage = 1; {CLEAR_FILTERS}; {RELOADING}; localStorage.setItem('kaunta.website', $selectedWebsite)"))
                                data-effect="window.kaunta.websites($_websites, $selectedWebsite)" {
                                option value="" { "Select a website" }
                            }
                            label for="days" { "Period" }
                            select id="days" data-bind="days" data-on:change=(format!("$breakdownPage = 1; {CLEAR_FILTERS}; {RELOADING}; localStorage.setItem('kaunta.days', $days)")) {
                                @for (days, label) in [("1", "Today"), ("7", "7 days"), ("30", "30 days"), ("90", "90 days")] {
                                    option value=(days) { (label) }
                                }
                            }
                        }
                    }
                    section id="kaunta-content" {
                        @match title {
                            "Websites" => { (websites()) },
                            "Goals" => { (goals()) },
                            "Map" => {
                                script src=(asset("/assets/vendor/d3-array.min.js")) {}
                                script src=(asset("/assets/vendor/d3-geo.min.js")) {}
                                script src=(asset("/assets/vendor/topojson-client.min.js")) {}
                                script src=(asset("/assets/js/map.js")) {}
                                div data-effect=(format!("if ($selectedWebsite) @get('/api/dashboard/map?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}))")) {}
                                (filter_chips())
                                h2 { "Visitors by country" }
                                a class="export-link" data-show="!!$selectedWebsite"
                                    data-attr:href=(format!("'/api/dashboard/export?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}) + '&type=countries'")) { "Export CSV" }
                                p { "Total visitors: " span data-text="$mapTotalVisitors" {} }
                                p data-show="$mapError" data-text="$mapError" role="alert" {}
                                div id="choropleth-map" data-effect="window.kaunta.choropleth($_mapData, $websitesLoading || (!!$selectedWebsite && $mapLoading), $mapError)" {}
                                div data-effect="window.kaunta.table('country-data', $_mapData, ['country_name', 'visitors', 'percentage'], $websitesLoading || (!!$selectedWebsite && $mapLoading), $mapError, {signal: '$filterCountry', key: 'country'})" {}
                                (table("country-data", &["Country", "Visitors", "Share (%)"]))
                            },
                            "Campaigns" => {
                                div data-effect="if ($selectedWebsite) @get('/api/dashboard/campaigns?' + window.kaunta.query($selectedWebsite, $days))" {}
                                a class="export-link" data-show="!!$selectedWebsite"
                                    data-attr:href="'/api/dashboard/export?' + window.kaunta.query($selectedWebsite, $days) + '&type=campaigns'" { "Export CSV" }
                                div class="campaign-grid" {
                                    @for dimension in ["source", "medium", "campaign", "term", "content"] {
                                        article class="campaign-card" data-dimension=(dimension) {
                                            h2 { "UTM " (dimension) }
                                            div id=(format!("utm-{dimension}-content")) class="empty-state-mini" {
                                                p { "Campaign visits will appear here." }
                                                small { "Add UTM tags to your shared links." }
                                            }
                                        }
                                    }
                                }
                            },
                            _ => { (overview()) },
                        }
                    }
                    noscript { "JavaScript is required to use the dashboard." }
                }
                footer { small { "Kaunta " (version) } }
            }
        }
    }
}

/// Active-filter chips; each clears its own filter and re-arms the loading
/// skeletons. Rendered on the pages whose queries honor the filters.
fn filter_chips() -> Markup {
    html! {
        div class="filter-chips" data-show="!!($filterCountry || $filterBrowser || $filterDevice || $filterPage)" {
            span { "Filtered to" }
            @for (signal, label) in [
                ("filterPage", "Page"),
                ("filterCountry", "Country"),
                ("filterBrowser", "Browser"),
                ("filterDevice", "Device"),
            ] {
                button type="button" class="filter-chip" data-show=(format!("!!${signal}"))
                    data-on:click=(format!("${signal} = ''; {RELOADING}")) {
                    (label) ": " span data-text=(format!("${signal}")) {} " ✕"
                }
            }
        }
    }
}

fn table(id: &str, headings: &[&str]) -> Markup {
    html! {
        table {
            thead { tr { @for heading in headings { th scope="col" { (heading) } } } }
            tbody id=(id) {}
        }
    }
}

fn overview() -> Markup {
    html! {
        p data-show="$statsError" data-text="$statsError" role="alert" {}
        p data-show="$chartError" data-text="$chartError" role="alert" {}
        p data-show="$breakdownError" data-text="$breakdownError" role="alert" {}
        div data-effect=(format!("if ($selectedWebsite) @get('/api/dashboard/stats?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}))")) {}
        div data-effect=(format!("if ($selectedWebsite) @get('/api/dashboard/timeseries?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}))")) {}
        (filter_chips())
        div class="portal-stats is-loading" aria-busy="true" data-attr:aria-busy="$websitesLoading || (!!$selectedWebsite && $statsLoading)"
            data-class:is-loading="$websitesLoading || (!!$selectedWebsite && $statsLoading)" {
            @for (label, signal) in [
                ("Online now", "$stats.current_visitors"),
                ("Pageviews", "$stats.today_pageviews"),
                ("Visitors", "$stats.today_visitors"),
                ("Bounce rate", "$stats.today_bounce_rate"),
            ] {
                article {
                    h2 { (label) }
                    strong data-text=(format!("$statsError || (!$websitesLoading && !$selectedWebsite) ? '—' : {signal}"))
                        data-attr:aria-hidden="$websitesLoading || (!!$selectedWebsite && $statsLoading)" {}
                }
            }
        }
        p class="portal-compare" data-show="!$websitesLoading && !!$selectedWebsite && !$statsLoading && !$statsError" {
            "Last " span data-text="$overview.period_days" {} " days: "
            strong data-text="$overview.visitors" {} " visitors "
            span data-text="window.kaunta.delta($overview.visitors, $overview.prev_visitors)" {}
            ", " strong data-text="$overview.pageviews" {} " pageviews "
            span data-text="window.kaunta.delta($overview.pageviews, $overview.prev_pageviews)" {}
            ", bounce " strong data-text="$overview.bounce_rate" {}
            " (prev " span data-text="$overview.prev_bounce_rate" {} ")"
        }
        h2 { "Live" }
        div id="live-feed" class="empty-state-mini" {
            p { "Waiting for visitors…" }
            small { "Tracked events stream here as they arrive." }
        }
        h2 { "Pageviews over time" }
        a class="export-link" data-show="!!$selectedWebsite"
            data-attr:href=(format!("'/api/dashboard/export?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}) + '&type=timeseries'")) { "Export CSV" }
        div id="pageviews-chart" data-effect="window.kaunta.chart($_timeseries, $websitesLoading || (!!$selectedWebsite && $chartLoading), $chartError)" {}
        details {
            summary { "View chart data" }
            div data-effect="window.kaunta.table('timeseries-data', $_timeseries, ['timestamp', 'value'], $websitesLoading || (!!$selectedWebsite && $chartLoading), $chartError)" {}
            (table("timeseries-data", &["Time (UTC)", "Pageviews"]))
        }
        h2 { "Breakdown" }
        a class="export-link" data-show="!!$selectedWebsite"
            data-attr:href=(format!("'/api/dashboard/export?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}) + '&type=breakdown&dimension=' + $breakdownType")) { "Export CSV" }
        label for="breakdown-type" { "Group by" }
        select id="breakdown-type" data-bind="breakdownType" data-on:change="$breakdownPage = 1; $breakdownLoading = true" {
            @for (kind, label) in [
                ("page", "Page"), ("entry_page", "Entry page"), ("exit_page", "Exit page"),
                ("source", "Source"), ("channel", "Channel"), ("referrer", "Referrer"),
                ("country", "Country"), ("city", "City"), ("region", "Region"),
                ("browser", "Browser"), ("os", "OS"), ("device", "Device"),
                ("event", "Event"),
            ] {
                option value=(kind) { (label) }
            }
        }
        div data-effect=(format!("if ($selectedWebsite) @get('/api/dashboard/breakdown?' + window.kaunta.query($selectedWebsite, $days, {FILTERS}) + '&type=' + $breakdownType + '&page_number=' + $breakdownPage)")) {}
        div data-effect="window.kaunta.breakdown($_breakdown.items, $websitesLoading || (!!$selectedWebsite && $breakdownLoading), $breakdownError, $breakdownType)" {}
        (table("breakdown-data", &["Name", "Count"]))
        button type="button" data-on:click="$breakdownPage--; $breakdownLoading = true" data-attr:disabled="$breakdownPage <= 1" { "Previous" }
        span data-text="$breakdownPage" {}
        button type="button" data-on:click="$breakdownPage++; $breakdownLoading = true" data-attr:disabled="$breakdownPage * 10 >= $_breakdown.total" { "Next" }
    }
}

fn websites() -> Markup {
    html! {
        div data-effect="window.kaunta.cards($_websites)" {}
        div data-effect="if ($websitesReload) { $websitesReload = false; @get('/api/dashboard/websites-init') }" {}
        div id="websites-container" {}
        h2 { "Add website" }
        form method="post" action="/api/dashboard/websites-create"
            data-on:submit__prevent="@post('/api/dashboard/websites-create', {contentType: 'form', headers: window.kaunta.headers()})" {
            label for="website-name" { "Name" }
            input id="website-name" name="name" data-bind="newWebsite.name";
            label for="website-domain" { "Domain" }
            input id="website-domain" name="domain" placeholder="example.com" required data-bind="newWebsite.domain";
            button type="submit" { "Add website" }
            p data-show="$createError" data-text="$createError" role="alert" {}
        }
        h2 { "Excluded traffic" }
        p {
            "Visits from these addresses are never recorded. To exclude a "
            "browser instead of an address, which is useful when your address "
            "changes, "
            "open any tracked page with "
            code { "?kaunta_ignore=true" }
            " (and " code { "?kaunta_ignore=false" } " to undo)."
        }
        div id="exclusions" {}
    }
}

fn goals() -> Markup {
    html! {
        div data-effect="if ($selectedWebsite) @get('/api/dashboard/goals?' + window.kaunta.query($selectedWebsite, $days))" {}
        div data-effect="window.kaunta.goals($_goals)" {}
        div id="goals-container" {}
        h2 data-text="$goalId ? 'Edit goal' : 'Add goal'" {}
        form class="goal-form" method="post" action="/api/dashboard/goals" data-class:is-editing="!!$goalId"
            data-on:submit__prevent="if ($goalId) { @put('/api/dashboard/goals/' + $goalId, {contentType: 'form', headers: window.kaunta.headers()}) } else { @post('/api/dashboard/goals', {contentType: 'form', headers: window.kaunta.headers()}) }" {
            input type="hidden" name="website_id" data-attr:value="$selectedWebsite";
            p class="edit-notice" data-show="!!$goalId" { "Editing an existing goal. Save your changes when ready." }
            div class="goal-fields" {
                div {
                    label for="goal-name" { "Name" }
                    input id="goal-name" name="name" placeholder="Newsletter signup" required data-bind="goalForm.name";
                }
                div {
                    label for="goal-type" { "Type" }
                    select id="goal-type" name="type" required data-bind="goalForm.type" {
                        option value="" { "Select a type" }
                        option value="page_view" { "Page view" }
                        option value="custom_event" { "Custom event" }
                    }
                }
                div {
                    label for="goal-value" { "Path or event name" }
                    input id="goal-value" name="value" placeholder="/thank-you or signup" required data-bind="goalForm.value";
                }
            }
            div class="goal-actions" {
                button type="submit" data-attr:disabled="!$selectedWebsite || $submitting" { "Save goal" }
                button type="button" data-show="!!$goalId" data-on:click="$goalId = ''; $goalForm = {name: '', type: 'page_view', value: ''}" { "Cancel edit" }
            }
            p data-show="$goalError" data-text="$goalError" role="alert" {}
        }
    }
}

#[cfg(test)]
mod tests;
