//! Action syntax validation for Datastar expressions.
//!
//! Validates @get, @post, @patch, @put, @delete SSE actions
//! and Pro actions like @clipboard, @fit.

use crate::helpers::{is_datastar_attr, ParsedTag};
use dictator_decree_abi::{Diagnostic, Diagnostics, Span};

/// SSE action names that require a URL argument.
const SSE_ACTIONS: &[&str] = &["@get", "@post", "@patch", "@put", "@delete"];

/// Pro actions.
const PRO_ACTIONS: &[&str] = &["@clipboard", "@fit"];

/// All known actions.
const ALL_ACTIONS: &[&str] = &[
    "@get",
    "@post",
    "@patch",
    "@put",
    "@delete",
    "@clipboard",
    "@fit",
];

/// Check action syntax in Datastar expressions.
pub fn check_actions(tag: &ParsedTag<'_>, diags: &mut Diagnostics) {
    for attr in &tag.attributes {
        if !is_datastar_attr(attr.name) {
            continue;
        }

        if let Some(value) = attr.value {
            check_action_syntax(value, attr, diags);
        }
    }
}

/// Check action syntax in a value expression.
fn check_action_syntax(
    value: &str,
    attr: &crate::helpers::ParsedAttribute<'_>,
    diags: &mut Diagnostics,
) {
    // Find all @ occurrences
    let bytes = value.as_bytes();
    let mut i = 0;

    while i < bytes.len() {
        if bytes[i] != b'@' {
            i += 1;
            continue;
        }

        // Extract action name
        let action_start = i;
        i += 1;
        while i < bytes.len() && is_action_char(bytes[i]) {
            i += 1;
        }
        let action_name = &value[action_start..i];

        if action_name.len() <= 1 {
            // Just @ without name
            continue;
        }

        // Check if it's a known action
        let is_sse = SSE_ACTIONS.contains(&action_name);
        let is_pro = PRO_ACTIONS.contains(&action_name);

        if !is_sse && !is_pro {
            // Unknown action - could be a typo
            if let Some(suggestion) = find_similar_action(action_name) {
                diags.push(Diagnostic {
                    rule: "datastar/action-syntax".to_string(),
                    message: format!(
                        "Unknown action '{}'. Did you mean '{}'?",
                        action_name, suggestion
                    ),
                    enforced: false,
                    span: Span::new(
                        attr.value_start.unwrap_or(attr.name_start),
                        attr.value_end.unwrap_or(attr.name_end),
                    ),
                });
            }
            continue;
        }

        // Skip whitespace
        while i < bytes.len() && bytes[i] == b' ' {
            i += 1;
        }

        // Check for parentheses
        if i >= bytes.len() || bytes[i] != b'(' {
            diags.push(Diagnostic {
                rule: "datastar/action-syntax".to_string(),
                message: format!(
                    "Action '{}' requires parentheses, e.g., {}('/path')",
                    action_name, action_name
                ),
                enforced: false,
                span: Span::new(
                    attr.value_start.unwrap_or(attr.name_start),
                    attr.value_end.unwrap_or(attr.name_end),
                ),
            });
            continue;
        }

        // Find matching closing paren
        let paren_start = i;
        let mut depth = 1;
        i += 1;
        while i < bytes.len() && depth > 0 {
            match bytes[i] {
                b'(' => depth += 1,
                b')' => depth -= 1,
                b'"' | b'\'' | b'`' => {
                    // Skip string content
                    let quote = bytes[i];
                    i += 1;
                    while i < bytes.len() && bytes[i] != quote {
                        if bytes[i] == b'\\' && i + 1 < bytes.len() {
                            i += 1;
                        }
                        i += 1;
                    }
                }
                _ => {}
            }
            i += 1;
        }

        if depth != 0 {
            diags.push(Diagnostic {
                rule: "datastar/action-syntax".to_string(),
                message: format!("Unclosed parentheses in '{}' call", action_name),
                enforced: false,
                span: Span::new(
                    attr.value_start.unwrap_or(attr.name_start),
                    attr.value_end.unwrap_or(attr.name_end),
                ),
            });
            continue;
        }

        // For SSE actions, check that the first argument looks like a URL
        if is_sse {
            let args = &value[paren_start + 1..i - 1];
            let first_arg = args.split(',').next().unwrap_or("").trim();

            if first_arg.is_empty() {
                diags.push(Diagnostic {
                    rule: "datastar/action-syntax".to_string(),
                    message: format!(
                        "SSE action '{}' requires a URL argument, e.g., {}('/api/endpoint')",
                        action_name, action_name
                    ),
                    enforced: false,
                    span: Span::new(
                        attr.value_start.unwrap_or(attr.name_start),
                        attr.value_end.unwrap_or(attr.name_end),
                    ),
                });
            } else if !looks_like_url(first_arg) && !looks_like_expression(first_arg) {
                diags.push(Diagnostic {
                    rule: "datastar/action-syntax".to_string(),
                    message: format!(
                        "SSE action '{}' URL should start with '/' or be a string/expression, got: {}",
                        action_name, first_arg
                    ),
                    enforced: false,
                    span: Span::new(
                        attr.value_start.unwrap_or(attr.name_start),
                        attr.value_end.unwrap_or(attr.name_end),
                    ),
                });
            }
        }
    }
}

/// Check if a byte is valid in an action name.
fn is_action_char(b: u8) -> bool {
    matches!(b, b'a'..=b'z' | b'A'..=b'Z')
}

/// Find a similar action name for typo suggestions.
fn find_similar_action(name: &str) -> Option<&'static str> {
    let lower = name.to_lowercase();

    for action in ALL_ACTIONS {
        if action.to_lowercase() == lower {
            return Some(action);
        }
        // Check for common typos
        if lower.contains(&action[1..]) {
            return Some(action);
        }
    }

    None
}

/// Check if a value looks like a URL (starts with / or is a quoted string starting with /).
fn looks_like_url(value: &str) -> bool {
    let trimmed = value.trim();

    if trimmed.starts_with('/') {
        return true;
    }

    // Check quoted strings
    if (trimmed.starts_with('\'') && trimmed.ends_with('\''))
        || (trimmed.starts_with('"') && trimmed.ends_with('"'))
        || (trimmed.starts_with('`') && trimmed.ends_with('`'))
    {
        let inner = &trimmed[1..trimmed.len() - 1];
        return inner.starts_with('/');
    }

    false
}

/// Check if a value looks like a JavaScript expression (variable, concatenation, etc.).
fn looks_like_expression(value: &str) -> bool {
    let trimmed = value.trim();

    // Contains $ (signal reference)
    if trimmed.contains('$') {
        return true;
    }

    // Contains + (string concatenation)
    if trimmed.contains('+') {
        return true;
    }

    // Template literal
    if trimmed.starts_with('`') {
        return true;
    }

    false
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::helpers::parse_tags;

    #[test]
    fn test_valid_sse_action() {
        let html = r#"<button data-on:click="@get('/api/data')">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_actions(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_action_missing_parens() {
        let html = r#"<button data-on:click="@get">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_actions(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("requires parentheses"));
    }

    #[test]
    fn test_action_empty_url() {
        let html = r#"<button data-on:click="@get()">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_actions(&tags[0], &mut diags);
        assert_eq!(diags.len(), 1);
        assert!(diags[0].message.contains("requires a URL"));
    }

    #[test]
    fn test_action_with_expression() {
        let html = r#"<button data-on:click="@get('/api/' + $endpoint)">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_actions(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }

    #[test]
    fn test_multiple_actions() {
        let html = r#"<div data-init="@get('/init')" data-on:click="@post('/submit')">"#;
        let tags = parse_tags(html);
        let mut diags = Diagnostics::new();
        check_actions(&tags[0], &mut diags);
        assert!(diags.is_empty());
    }
}
