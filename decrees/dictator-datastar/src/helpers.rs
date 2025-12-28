//! HTML parsing utilities for Datastar attribute extraction.

/// Parsed HTML attribute with position information.
#[derive(Debug, Clone)]
pub struct ParsedAttribute<'a> {
    /// Attribute name (e.g., "data-show", "data-on:click")
    pub name: &'a str,
    /// Attribute value if present
    pub value: Option<&'a str>,
    /// Byte offset of attribute name start
    pub name_start: usize,
    /// Byte offset of attribute name end
    pub name_end: usize,
    /// Byte offset of value start (if present)
    pub value_start: Option<usize>,
    /// Byte offset of value end (if present)
    pub value_end: Option<usize>,
}

/// Parsed HTML tag with its attributes.
#[derive(Debug)]
pub struct ParsedTag<'a> {
    /// Tag name (e.g., "div", "button", "template")
    pub name: &'a str,
    /// Parsed attributes
    pub attributes: Vec<ParsedAttribute<'a>>,
}

/// Check if byte is whitespace.
#[inline]
pub fn is_space(b: u8) -> bool {
    matches!(b, b' ' | b'\n' | b'\t' | b'\r' | b'\x0c')
}

/// Check if byte is valid in a tag name.
#[inline]
pub fn is_tag_name_char(b: u8) -> bool {
    matches!(b, b'a'..=b'z' | b'A'..=b'Z' | b'0'..=b'9' | b'-' | b':' | b'_')
}

/// Parse all HTML tags from source, yielding tags with their attributes.
pub fn parse_tags(source: &str) -> Vec<ParsedTag<'_>> {
    let mut tags = Vec::new();
    let bytes = source.as_bytes();
    let mut i = 0;

    while i < bytes.len() {
        if bytes[i] != b'<' {
            i += 1;
            continue;
        }

        // Skip HTML comments
        if i + 3 < bytes.len()
            && bytes[i + 1] == b'!'
            && bytes[i + 2] == b'-'
            && bytes[i + 3] == b'-'
        {
            if let Some(end) = source[i + 4..].find("-->") {
                i = i + 4 + end + 3;
                continue;
            }
            break;
        }

        let mut idx = i + 1;

        // Skip closing tag slash
        if idx < bytes.len() && bytes[idx] == b'/' {
            idx += 1;
        }

        // Skip whitespace
        while idx < bytes.len() && is_space(bytes[idx]) {
            idx += 1;
        }

        // Skip DOCTYPE, CDATA, etc.
        if idx < bytes.len() && (bytes[idx] == b'!' || bytes[idx] == b'?') {
            if let Some(end) = source[idx..].find('>') {
                i = idx + end + 1;
                continue;
            }
            break;
        }

        // Parse tag name
        let tag_name_start = idx;
        while idx < bytes.len() && is_tag_name_char(bytes[idx]) {
            idx += 1;
        }
        let tag_name = &source[tag_name_start..idx];

        if tag_name.is_empty() {
            i += 1;
            continue;
        }

        // Parse attributes
        let mut attributes = Vec::new();

        loop {
            // Skip whitespace
            while idx < bytes.len() && is_space(bytes[idx]) {
                idx += 1;
            }

            if idx >= bytes.len() {
                break;
            }

            let b = bytes[idx];

            // End of tag
            if b == b'>' {
                idx += 1;
                break;
            }

            // Self-closing
            if b == b'/' {
                idx += 1;
                if idx < bytes.len() && bytes[idx] == b'>' {
                    idx += 1;
                }
                break;
            }

            // Parse attribute name
            let attr_start = idx;
            while idx < bytes.len()
                && !is_space(bytes[idx])
                && bytes[idx] != b'='
                && bytes[idx] != b'>'
                && bytes[idx] != b'/'
            {
                idx += 1;
            }
            let attr_end = idx;

            if attr_end == attr_start {
                idx += 1;
                continue;
            }

            let name = &source[attr_start..attr_end];

            // Skip whitespace before =
            while idx < bytes.len() && is_space(bytes[idx]) {
                idx += 1;
            }

            // Parse value if present
            let mut value = None;
            let mut value_start = None;
            let mut value_end = None;

            if idx < bytes.len() && bytes[idx] == b'=' {
                idx += 1;

                // Skip whitespace after =
                while idx < bytes.len() && is_space(bytes[idx]) {
                    idx += 1;
                }

                if idx < bytes.len() {
                    if bytes[idx] == b'"' || bytes[idx] == b'\'' {
                        let quote = bytes[idx];
                        idx += 1;
                        let val_start = idx;
                        while idx < bytes.len() && bytes[idx] != quote {
                            idx += 1;
                        }
                        value = Some(&source[val_start..idx]);
                        value_start = Some(val_start);
                        value_end = Some(idx);
                        if idx < bytes.len() && bytes[idx] == quote {
                            idx += 1;
                        }
                    } else {
                        // Unquoted value
                        let val_start = idx;
                        while idx < bytes.len() && !is_space(bytes[idx]) && bytes[idx] != b'>' {
                            idx += 1;
                        }
                        value = Some(&source[val_start..idx]);
                        value_start = Some(val_start);
                        value_end = Some(idx);
                    }
                }
            }

            attributes.push(ParsedAttribute {
                name,
                value,
                name_start: attr_start,
                name_end: attr_end,
                value_start,
                value_end,
            });
        }

        tags.push(ParsedTag {
            name: tag_name,
            attributes,
        });

        i = idx;
    }

    tags
}

/// Check if an attribute is a Datastar attribute.
#[inline]
pub fn is_datastar_attr(name: &str) -> bool {
    name.starts_with("data-")
}

/// Extract the base attribute name without modifiers.
/// e.g., "data-on:click__debounce.500ms" -> "data-on:click"
pub fn base_attr_name(name: &str) -> &str {
    if let Some(pos) = name.find("__") {
        &name[..pos]
    } else {
        name
    }
}

/// Extract modifiers from attribute name.
/// e.g., "data-on:click__debounce.500ms__once" -> ["debounce.500ms", "once"]
pub fn extract_modifiers(name: &str) -> Vec<&str> {
    let mut modifiers = Vec::new();
    let mut remaining = name;

    while let Some(pos) = remaining.find("__") {
        remaining = &remaining[pos + 2..];
        if let Some(next_pos) = remaining.find("__") {
            modifiers.push(&remaining[..next_pos]);
        } else {
            if !remaining.is_empty() {
                modifiers.push(remaining);
            }
            break;
        }
    }

    modifiers
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_simple_tag() {
        let html = r#"<div data-show="$visible">Hello</div>"#;
        let tags = parse_tags(html);
        assert_eq!(tags.len(), 2); // div and /div
        assert_eq!(tags[0].name, "div");
        assert_eq!(tags[0].attributes.len(), 1);
        assert_eq!(tags[0].attributes[0].name, "data-show");
        assert_eq!(tags[0].attributes[0].value, Some("$visible"));
    }

    #[test]
    fn test_parse_multiple_attributes() {
        let html = r#"<button data-on:click="$foo = 1" data-class:active="$bar">"#;
        let tags = parse_tags(html);
        assert_eq!(tags[0].attributes.len(), 2);
    }

    #[test]
    fn test_base_attr_name() {
        assert_eq!(
            base_attr_name("data-on:click__debounce.500ms"),
            "data-on:click"
        );
        assert_eq!(base_attr_name("data-show"), "data-show");
    }

    #[test]
    fn test_extract_modifiers() {
        let mods = extract_modifiers("data-on:click__debounce.500ms__once");
        assert_eq!(mods, vec!["debounce.500ms", "once"]);
    }

    #[test]
    fn test_is_datastar_attr() {
        assert!(is_datastar_attr("data-show"));
        assert!(is_datastar_attr("data-on:click"));
        assert!(!is_datastar_attr("class"));
        assert!(!is_datastar_attr("id"));
    }
}
