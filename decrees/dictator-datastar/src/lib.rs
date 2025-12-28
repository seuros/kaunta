#![warn(rust_2024_compatibility, clippy::all)]
#![allow(unsafe_attr_outside_unsafe, unsafe_op_in_unsafe_fn)]

//! # dictator-datastar
//!
//! Datastar hygiene decree for the Dictator structural linter.
//! Validates Datastar HTML attributes for syntax and best practices.
//!
//! ## Rules
//!
//! - `datastar/no-alpine-vue-attrs` - Disallows Alpine.js/Vue.js style attributes
//! - `datastar/require-value` - Requires values for expression-based attributes
//! - `datastar/for-template` - Requires data-for on <template> elements
//! - `datastar/typo` - Detects common typos in attribute names
//! - `datastar/invalid-modifier` - Validates modifier syntax
//! - `datastar/action-syntax` - Validates @action syntax
//!
//! ## Note on Attribute Order
//!
//! Datastar processes attributes in DOM order (depth-first, then attribute order).
//! The order is **semantic**, not stylistic - dependencies between attributes
//! require specific ordering. This decree does NOT enforce attribute ordering
//! since correct order depends on the specific use case.
//!
//! ## Building
//!
//! ```bash
//! cargo build --release --target wasm32-wasip1
//! ```

mod actions;
mod config;
mod helpers;
mod modifiers;
mod typos;
mod validation;

use config::DatastarConfig;
use dictator_decree_abi::{Decree, DecreeMetadata, Diagnostics};
use helpers::parse_tags;

/// Datastar hygiene decree - enforces Datastar best practices.
#[derive(Default)]
pub struct DatastarHygiene {
    config: DatastarConfig,
}

impl DatastarHygiene {
    /// Create a new DatastarHygiene decree with default config.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Create a new DatastarHygiene decree with custom config.
    #[must_use]
    pub const fn with_config(config: DatastarConfig) -> Self {
        Self { config }
    }
}

impl Decree for DatastarHygiene {
    fn name(&self) -> &str {
        "datastar"
    }

    fn lint(&self, _path: &str, source: &str) -> Diagnostics {
        let mut diags = Diagnostics::new();

        // Parse HTML tags
        let tags = parse_tags(source);

        for tag in &tags {
            // Check for Alpine/Vue attributes
            if self.config.check_alpine_vue {
                validation::check_alpine_vue(tag, &mut diags);
            }

            // Check required values
            if self.config.check_required_values {
                validation::check_required_values(tag, &mut diags);
            }

            // Check data-for on template
            if self.config.check_for_template {
                validation::check_for_on_template(tag, &mut diags);
            }

            // Check for typos
            if self.config.check_typos {
                typos::check_typos(tag, &mut diags);
            }

            // Check modifier syntax
            if self.config.check_modifiers {
                modifiers::check_modifiers(tag, &mut diags);
            }

            // Check action syntax
            if self.config.check_actions {
                actions::check_actions(tag, &mut diags);
            }
        }

        diags
    }

    fn metadata(&self) -> DecreeMetadata {
        DecreeMetadata {
            abi_version: dictator_decree_abi::ABI_VERSION.to_string(),
            decree_version: env!("CARGO_PKG_VERSION").to_string(),
            description: "Datastar HTML attribute hygiene and best practices".to_string(),
            dectauthors: Some(env!("CARGO_PKG_AUTHORS").to_string()),
            supported_extensions: vec!["html".to_string(), "htm".to_string()],
            supported_filenames: vec![],
            skip_filenames: vec![],
            capabilities: vec![dictator_decree_abi::Capability::Lint],
        }
    }
}

/// Factory for creating decree instance.
#[must_use]
pub fn init_decree() -> Box<dyn Decree> {
    Box::new(DatastarHygiene::default())
}

// =============================================================================
// WASM COMPONENT BINDINGS
// =============================================================================

wit_bindgen::generate!({
    path: "wit/decree.wit",
    world: "decree",
});

struct PluginImpl;

impl exports::dictator::decree::lints::Guest for PluginImpl {
    fn name() -> String {
        DatastarHygiene::default().name().to_string()
    }

    fn lint(path: String, source: String) -> Vec<exports::dictator::decree::lints::Diagnostic> {
        let decree = DatastarHygiene::default();
        let diags = decree.lint(&path, &source);
        diags
            .into_iter()
            .map(|d| exports::dictator::decree::lints::Diagnostic {
                rule: d.rule,
                message: d.message,
                severity: if d.enforced {
                    exports::dictator::decree::lints::Severity::Info
                } else {
                    exports::dictator::decree::lints::Severity::Error
                },
                span: exports::dictator::decree::lints::Span {
                    start: d.span.start as u32,
                    end: d.span.end as u32,
                },
            })
            .collect()
    }

    fn metadata() -> exports::dictator::decree::lints::DecreeMetadata {
        let decree = DatastarHygiene::default();
        let meta = decree.metadata();
        exports::dictator::decree::lints::DecreeMetadata {
            abi_version: meta.abi_version,
            decree_version: meta.decree_version,
            description: meta.description,
            dectauthors: meta.dectauthors,
            supported_extensions: meta.supported_extensions,
            supported_filenames: meta.supported_filenames,
            skip_filenames: meta.skip_filenames,
            capabilities: meta
                .capabilities
                .into_iter()
                .map(|c| match c {
                    dictator_decree_abi::Capability::Lint => {
                        exports::dictator::decree::lints::Capability::Lint
                    }
                    dictator_decree_abi::Capability::AutoFix => {
                        exports::dictator::decree::lints::Capability::AutoFix
                    }
                    dictator_decree_abi::Capability::Streaming => {
                        exports::dictator::decree::lints::Capability::Streaming
                    }
                    dictator_decree_abi::Capability::RuntimeConfig => {
                        exports::dictator::decree::lints::Capability::RuntimeConfig
                    }
                    dictator_decree_abi::Capability::RichDiagnostics => {
                        exports::dictator::decree::lints::Capability::RichDiagnostics
                    }
                })
                .collect(),
        }
    }
}

export!(PluginImpl);

// =============================================================================
// TESTS
// =============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_full_lint() {
        let decree = DatastarHygiene::default();
        let html = r#"
            <div data-signals:count="0"
                 data-show="$count > 0"
                 data-on:click="$count++">
                Count: <span data-text="$count"></span>
            </div>
        "#;
        let diags = decree.lint("test.html", html);
        assert!(
            diags.is_empty(),
            "Expected no diagnostics, got: {:?}",
            diags
        );
    }

    #[test]
    fn test_detects_alpine_attrs() {
        let decree = DatastarHygiene::default();
        let html = r#"<div x-show="visible" @click="handle()">"#;
        let diags = decree.lint("test.html", html);
        assert_eq!(diags.len(), 2);
        assert!(diags
            .iter()
            .all(|d| d.rule == "datastar/no-alpine-vue-attrs"));
    }

    #[test]
    fn test_detects_typo() {
        let decree = DatastarHygiene::default();
        let html = r#"<div data-intersects="@get('/foo')">"#;
        let diags = decree.lint("test.html", html);
        assert!(diags.iter().any(|d| d.rule == "datastar/typo"));
    }

    #[test]
    fn test_metadata() {
        let decree = DatastarHygiene::default();
        let meta = decree.metadata();
        assert_eq!(meta.supported_extensions, vec!["html", "htm"]);
        assert!(meta
            .capabilities
            .contains(&dictator_decree_abi::Capability::Lint));
    }
}
