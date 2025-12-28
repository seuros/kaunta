//! Modifier validation for Datastar attributes.

use crate::helpers::{base_attr_name, extract_modifiers, is_datastar_attr, ParsedTag};
use dictator_decree_abi::{Diagnostic, Diagnostics, Span};

/// Valid modifiers for data-on:* event handlers.
const EVENT_MODIFIERS: &[&str] = &[
    "once",
    "passive",
    "capture",
    "case",
    "delay",
    "debounce",
    "throttle",
    "viewtransition",
    "window",
    "outside",
    "prevent",
    "stop",
];

/// Valid modifiers for data-on-intersect.
const INTERSECT_MODIFIERS: &[&str] = &[
    "once",
    "exit",
    "half",
    "full",
    "threshold",
    "delay",
    "debounce",
    "throttle",
    "viewtransition",
];

/// Valid modifiers for data-persist.
const PERSIST_MODIFIERS: &[&str] = &["session"];

/// Valid modifiers for data-init.
const INIT_MODIFIERS: &[&str] = &["delay", "viewtransition"];

/// Valid casing modifiers (apply to many attributes).
const CASE_MODIFIERS: &[&str] = &["camel", "kebab", "snake", "pascal"];

/// Check modifier validity for Datastar attributes.
pub fn check_modifiers(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        if !is_datastar_attr(attr.name) {
            continue;
        }

        let modifiers = extract_modifiers(attr.name);
        if modifiers.is_empty() {
            continue;
        }

        let base = base_attr_name(attr.name);
        let valid_modifiers = get_valid_modifiers(base);

        for modifier in modifiers {
            // Extract base modifier name (without timing value like .500ms)
            let mod_base = modifier.split('.').next().unwrap_or(modifier);

            // Check if it's a case modifier
            if mod_base == "case" {
                // Validate case modifier value
                if let Some(case_value) = modifier.strip_prefix("case.") {
                    if !CASE_MODIFIERS.contains(&case_value) {
                        diags.push(Diagnostic {
                            rule: "datastar/invalid-modifier".to_string(),
                            message: format!(
                                "Invalid case modifier '{}'. Valid options: camel, kebab, snake, pascal",
                                case_value
                            ),
                            enforced: false,
                            span: Span::new(attr.name_start, attr.name_end),
                        });
                    }
                }
                continue;
            }

            // Check if modifier is valid for this attribute
            if !valid_modifiers.contains(&mod_base) && !is_timing_modifier(mod_base) {
                diags.push(Diagnostic {
                    rule: "datastar/invalid-modifier".to_string(),
                    message: format!(
                        "Invalid modifier '{}' for '{}'. Valid modifiers: {}",
                        modifier,
                        base,
                        valid_modifiers.join(", ")
                    ),
                    enforced: false,
                    span: Span::new(attr.name_start, attr.name_end),
                });
            }
        }
    }
}

/// Get valid modifiers for an attribute.
fn get_valid_modifiers(base_attr: &str) -> &'static [&'static str] {
    if base_attr.starts_with("data-on:") {
        EVENT_MODIFIERS
    } else if base_attr == "data-on-intersect" {
        INTERSECT_MODIFIERS
    } else if base_attr == "data-persist" {
        PERSIST_MODIFIERS
    } else if base_attr == "data-init" {
        INIT_MODIFIERS
    } else if base_attr == "data-on-interval" || base_attr == "data-on-signal-patch" {
        // Similar to event modifiers
        &["delay", "debounce", "throttle", "viewtransition"]
    } else if base_attr == "data-on-raf" || base_attr == "data-on-resize" {
        &["debounce", "throttle"]
    } else if base_attr == "data-effect" {
        &["viewtransition"]
    } else if base_attr.starts_with("data-signals")
        || base_attr.starts_with("data-computed")
        || base_attr.starts_with("data-ref")
        || base_attr.starts_with("data-bind")
        || base_attr.starts_with("data-indicator")
    {
        // Only case modifiers
        &["case"]
    } else {
        &[]
    }
}

/// Check if a modifier is a timing value (e.g., "500ms", "1s", "leading", "trailing").
fn is_timing_modifier(modifier: &str) -> bool {
    modifier.ends_with("ms")
        || modifier.ends_with('s')
        || modifier == "leading"
        || modifier == "trailing"
        || modifier == "notrailing"
        || modifier == "noleading"
        || modifier.parse::<f64>().is_ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::helpers::parse_tags;

    #[test]
    fn test_valid_event_modifiers() {
        let html = r#"<div data-on:click__debounce.500ms__once="handle()">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_modifiers(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_invalid_event_modifier() {
        let html = r#"<div data-on:click__invalid="handle()">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_modifiers(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("invalid"));
    }

    #[test]
    fn test_valid_persist_modifier() {
        let html = r#"<div data-persist__session>"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_modifiers(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_valid_case_modifier() {
        let html = r#"<div data-signals:my-var__case.kebab="1">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_modifiers(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_invalid_case_modifier() {
        let html = r#"<div data-signals:my-var__case.invalid="1">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_modifiers(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("Invalid case modifier"));
    }
}
