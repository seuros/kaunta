use crate::db::backup::{BackupMode, create_backup};
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct BackupParams {
    /// "full" (pg_dump of the whole database) or "period" (tar.gz of
    /// events, sessions, and goal completions for the last `days`).
    pub mode: String,
    /// Window for period mode (default 7); ignored for full.
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct BackupOutput {
    pub file_name: String,
    /// Download it from this path on the same host serving this MCP
    /// endpoint. The sha256 in the name is the access capability.
    pub download_path: String,
    pub sha256: String,
    pub size_bytes: u64,
    /// The server deletes the file after roughly one hour.
    pub expires_at: String,
    pub mode: String,
}

impl KauntaMcp {
    /// Generate a database backup and expose it for download for one hour
    /// at /backups/<file>, where the sha256-suffixed name is the access
    /// capability. Operator sessions only.
    #[mcp_tool(
        name = "create_backup",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "BackupOutput"
    )]
    async fn create_backup(&self, _ctx: Ctx<'_>, params: Parameters<BackupParams>) -> ToolResult {
        let BackupParams { mode, days } = params.0;
        let backup_mode = match mode.as_str() {
            "full" => BackupMode::Full,
            "period" => {
                crate::mcp::validate_days(days)?;
                let to = OffsetDateTime::now_utc();
                BackupMode::Period {
                    from: to - time::Duration::days(i64::from(days)),
                    to,
                }
            }
            other => {
                return Err(ToolError::InvalidArguments(format!(
                    "unknown backup mode {other:?}; expected full or period"
                )));
            }
        };
        let backup = create_backup(
            &self.pool,
            &self.database_url,
            &self.backups_dir(),
            backup_mode,
            env!("CARGO_PKG_VERSION"),
        )
        .await
        .map_err(|error| ToolError::Execution(error.to_string()))?;
        let expires_at = (OffsetDateTime::now_utc() + time::Duration::hours(1))
            .format(&Rfc3339)
            .map_err(|error| ToolError::Internal(error.to_string()))?;
        structured(BackupOutput {
            download_path: format!("/backups/{}", backup.file_name),
            file_name: backup.file_name,
            sha256: backup.sha256,
            size_bytes: backup.size_bytes,
            expires_at,
            mode,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::create_backup_tool_info(),
        KauntaMcp::create_backup_handler,
        None,
    )
}
