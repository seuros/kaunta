//! Configuration for the Datastar decree.

/// Configuration options for Datastar linting.
#[derive(Debug, Clone)]
pub struct DatastarConfig {
    /// Check for Alpine/Vue attributes
    pub check_alpine_vue: bool,
    /// Check for required values
    pub check_required_values: bool,
    /// Check for typos in attribute names
    pub check_typos: bool,
    /// Check modifier syntax
    pub check_modifiers: bool,
    /// Check action syntax (@get, @post, etc.)
    pub check_actions: bool,
    /// Check data-for on template elements
    pub check_for_template: bool,
}

impl Default for DatastarConfig {
    fn default() -> Self {
        Self {
            check_alpine_vue: true,
            check_required_values: true,
            check_typos: true,
            check_modifiers: true,
            check_actions: true,
            check_for_template: true,
        }
    }
}
