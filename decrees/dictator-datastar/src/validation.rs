//! Value and expression validation for Datastar attributes.

use crate::helpers::{base_attr_name, ParsedTag};
use dictator_decree_abi::{Diagnostic, Diagnostics, Span};

/// Check for Alpine.js or Vue.js style attributes.
pub fn check_alpine_vue(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        if is_alpine_or_vue_attr(attr.name) {
            diags.push(Diagnostic {
                rule: "datastar/no-alpine-vue-attrs".to_string(),
                message: format!("Disallowed Alpine/Vue-style attribute: {}", attr.name),
                enforced: false,
                span: Span::new(attr.name_start, attr.name_end),
            });
        }
    }
}

/// Check if an attribute looks like Alpine.js or Vue.js syntax.
fn is_alpine_or_vue_attr(name: &str) -> bool {
    name.starts_with("x-")
        || name.starts_with("x:")
        || name.starts_with("v-")
        || name.starts_with('@')
        || name.starts_with(':')
}

/// Check that required Datastar attributes have values.
pub fn check_required_values(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        if requires_value(attr.name) {
            let has_value = attr.value.map(|v| !v.is_empty()).unwrap_or(false);
            if !has_value {
                diags.push(Diagnostic {
                    rule: "datastar/require-value".to_string(),
                    message: format!("Datastar attribute '{}' requires a value", attr.name),
                    enforced: false,
                    span: Span::new(attr.name_start, attr.name_end),
                });
            }
        }
    }
}

/// Check if an attribute requires a value.
fn requires_value(name: &str) -> bool {
    let base = base_attr_name(name);
    matches!(
        base,
        "data-show"
            | "data-text"
            | "data-html"
            | "data-class"
            | "data-effect"
            | "data-computed"
            | "data-replace-url"
    ) || base.starts_with("data-on:")
        || base.starts_with("data-attr:")
        || base.starts_with("data-class:")
        || base.starts_with("data-style:")
        || base.starts_with("data-computed:")
}

/// Check that data-for is on a template element.
pub fn check_for_on_template(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        if attr.name == "data-for" && tag.name.to_lowercase() != "template" {
            diags.push(Diagnostic {
                rule: "datastar/for-template".to_string(),
                message: format!(
                    "data-for must be on a <template> element, found on <{}>",
                    tag.name
                ),
                enforced: false,
                span: Span::new(attr.name_start, attr.name_end),
            });
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::helpers::parse_tags;

    #[test]
    fn test_alpine_vue_detection() {
        let html = r#"<div x-show="visible" v-if="test" @click="handle" :class="foo">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_alpine_vue(&tags[0], &mut diags);
        assert_eq!(diags.len(), 4);
    }

    #[test]
    fn test_required_value_missing() {
        let html = r#"<div data-show data-text="">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_required_values(&tags[0], &mut diags);
        assert_eq!(diags.len(), 2);
    }

    #[test]
    fn test_for_on_template() {
        let html = r#"<div data-for="item in $items">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_for_on_template(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("template"));
    }

    #[test]
    fn test_for_on_template_valid() {
        let html = r#"<template data-for="item in $items">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_for_on_template(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }
}
