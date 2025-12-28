//! Typo detection for Datastar attributes.

use crate::helpers::ParsedTag;
use dictator_decree_abi::{Diagnostic, Diagnostics, Span};

/// Common typos and their corrections.
const TYPOS: &[(&str, &str)] = &[
    // Wrong separator (hyphen vs colon)
    ("data-on-click", "data-on:click"),
    ("data-on-submit", "data-on:submit"),
    ("data-on-input", "data-on:input"),
    ("data-on-change", "data-on:change"),
    ("data-on-keydown", "data-on:keydown"),
    ("data-on-keyup", "data-on:keyup"),
    ("data-on-focus", "data-on:focus"),
    ("data-on-blur", "data-on:blur"),
    ("data-on-mouseenter", "data-on:mouseenter"),
    ("data-on-mouseleave", "data-on:mouseleave"),
    ("data-bind-value", "data-bind:value"),
    ("data-bind-checked", "data-bind:checked"),
    ("data-attr-disabled", "data-attr:disabled"),
    ("data-attr-href", "data-attr:href"),
    ("data-class-active", "data-class:active"),
    ("data-style-color", "data-style:color"),
    // Common misspellings
    ("data-intersects", "data-on-intersect"),
    ("data-intersect", "data-on-intersect"),
    ("data-onload", "data-on:load or data-init"),
    ("data-onclick", "data-on:click"),
    ("data-onsubmit", "data-on:submit"),
    // Wrong pluralization
    ("data-signal", "data-signals"),
    // Old/wrong names
    ("data-visible", "data-show"),
    ("data-hidden", "data-show (with negation)"),
    ("data-content", "data-text or data-html"),
    ("data-value", "data-bind"),
    ("data-model", "data-bind"),
    // Vue/Alpine confusion
    ("data-if", "data-show"),
    ("data-else", "data-show (with negation)"),
    ("data-v-show", "data-show"),
    ("data-v-if", "data-show"),
    ("data-x-show", "data-show"),
    ("data-x-if", "data-show"),
];

/// Check for common typos in Datastar attribute names.
pub fn check_typos(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        // Only check data- prefixed attributes
        if !attr.name.starts_with("data-") {
            continue;
        }

        // Extract base name (without modifiers)
        let base_name = if let Some(pos) = attr.name.find("__") {
            &attr.name[..pos]
        } else {
            attr.name
        };

        // Check against known typos
        let mut found_typo = false;
        for (typo, suggestion) in TYPOS {
            if base_name == *typo {
                diags.push(Diagnostic {
                    rule: "datastar/typo".to_string(),
                    message: format!("Possible typo: '{}' - did you mean '{}'?", typo, suggestion),
                    enforced: false,
                    span: Span::new(attr.name_start, attr.name_end),
                });
                found_typo = true;
                break;
            }
        }

        // Skip further checks if we already found a typo from the known list
        if found_typo {
            continue;
        }

        // Check for hyphen where colon expected (data-on-* should be data-on:*)
        if base_name.starts_with("data-on-") && !is_valid_hyphen_event(base_name) {
            let event_name = &base_name[8..]; // after "data-on-"
            diags.push(Diagnostic {
                rule: "datastar/typo".to_string(),
                message: format!(
                    "Use colon for events: 'data-on:{}' instead of 'data-on-{}'",
                    event_name, event_name
                ),
                enforced: false,
                span: Span::new(attr.name_start, attr.name_end),
            });
        }

        // Check for hyphen where colon expected in other prefixes
        check_prefix_separator(attr, "data-bind-", "data-bind:", diags);
        check_prefix_separator(attr, "data-attr-", "data-attr:", diags);
        check_prefix_separator(attr, "data-class-", "data-class:", diags);
        check_prefix_separator(attr, "data-style-", "data-style:", diags);
        check_prefix_separator(attr, "data-indicator-", "data-indicator:", diags);
    }
}

/// Check if a data-on-* attribute is a valid hyphenated event (not a typo).
fn is_valid_hyphen_event(name: &str) -> bool {
    matches!(
        name,
        "data-on-intersect"
            | "data-on-interval"
            | "data-on-signal-patch"
            | "data-on-raf"
            | "data-on-resize"
            | "data-on-load"
    )
}

/// Check for wrong separator in prefixed attributes.
fn check_prefix_separator(
    attr: &crate::helpers::ParsedAttribute<'_>,
    wrong_prefix: &str,
    correct_prefix: &str,
    diags: &mut Diagnostics,
) {
    let base_name = if let Some(pos) = attr.name.find("__") {
        &attr.name[..pos]
    } else {
        attr.name
    };

    if base_name.starts_with(wrong_prefix) {
        let suffix = &base_name[wrong_prefix.len()..];
        diags.push(Diagnostic {
            rule: "datastar/typo".to_string(),
            message: format!(
                "Use colon separator: '{}{}' instead of '{}{}'",
                correct_prefix, suffix, wrong_prefix, suffix
            ),
            enforced: false,
            span: Span::new(attr.name_start, attr.name_end),
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::helpers::parse_tags;

    #[test]
    fn test_detect_intersects_typo() {
        let html = r#"<div data-intersects="@get('/foo')">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_typos(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("data-on-intersect"));
    }

    #[test]
    fn test_detect_wrong_separator() {
        let html = r#"<div data-on-click="$foo = 1">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_typos(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("data-on:click"));
    }

    #[test]
    fn test_valid_hyphen_events() {
        let html = r#"<div data-on-intersect="@get('/foo')" data-on-interval="tick()">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_typos(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_correct_attributes() {
        let html = r#"<div data-on:click="$foo = 1" data-show="$visible">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_typos(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }
}
