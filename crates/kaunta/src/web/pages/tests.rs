use super::*;

#[test]
fn login_posts_credentials_and_loads_runtime() {
    let page = login("test").into_string();
    assert!(page.contains("method=\"post\""));
    assert!(page.contains("/assets/js/portal.js"));
    assert!(page.contains("/assets/datastar.js"));
    assert!(!page.contains("@get('/api/auth/login')"));
}

#[test]
fn every_dashboard_initializes_and_renders_content() {
    for title in ["Dashboard", "Map", "Campaigns", "Websites", "Goals"] {
        let page = dashboard(title, "test").into_string();
        assert!(page.contains("data-init="), "{title}");
        assert!(page.contains("data-effect="), "{title}");
        assert!(page.contains("Sign out"), "{title}");
        assert!(!page.contains("<section id=\"kaunta-content\"></section>"));
    }
}

#[test]
fn export_links_follow_selected_website_and_period() {
    for (title, kinds) in [
        ("Dashboard", &["timeseries", "breakdown"][..]),
        ("Map", &["countries"][..]),
        ("Campaigns", &["campaigns"][..]),
    ] {
        let page = dashboard(title, "test").into_string();
        for kind in kinds {
            assert!(
                page.contains(&format!("&amp;type={kind}")),
                "{title}: {kind}"
            );
        }
        assert!(page.contains("data-show=\"!!$selectedWebsite\""), "{title}");
        assert!(
            page.contains("window.kaunta.query($selectedWebsite, $days"),
            "{title}"
        );
    }
}
