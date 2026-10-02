use include_dir::{Dir, include_dir};
use rama::http::{
    Request, Response, StatusCode, header,
    service::web::{
        extract::Path,
        response::{DatastarScript, DatastarSourceMap, IntoResponse},
    },
};
use serde::Deserialize;
use sha2::{Digest, Sha256};

use crate::web::http::{bytes_response, header_string, set_header};

static ASSETS: Dir<'_> = include_dir!("$CARGO_MANIFEST_DIR/../../assets");
static TRACKER: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../tracker/kaunta.js"
));

#[derive(Debug, Deserialize)]
pub struct AssetPath {
    path: String,
}

pub async fn tracker(request: Request) -> Response {
    let digest = Sha256::digest(TRACKER);
    let etag = format!("\"{}\"", hex::encode(&digest[..8]));
    let mut response = if header_string(request.headers(), header::IF_NONE_MATCH) == etag {
        StatusCode::NOT_MODIFIED.into_response()
    } else {
        bytes_response(
            StatusCode::OK,
            "application/javascript; charset=utf-8",
            TRACKER,
        )
    };
    set_header(&mut response, header::ETAG, &etag);
    set_header(
        &mut response,
        header::CACHE_CONTROL,
        "public, max-age=3600, immutable",
    );
    set_header(&mut response, header::ACCESS_CONTROL_ALLOW_ORIGIN, "*");
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_METHODS,
        "GET, OPTIONS",
    );
    set_header(&mut response, header::VARY, "Origin");
    set_header(
        &mut response,
        rama::http::HeaderName::from_static("timing-allow-origin"),
        "*",
    );
    set_header(
        &mut response,
        rama::http::HeaderName::from_static("x-content-type-options"),
        "nosniff",
    );
    set_header(
        &mut response,
        rama::http::HeaderName::from_static("x-frame-options"),
        "DENY",
    );
    response
}

pub async fn asset(Path(path): Path<AssetPath>) -> Response {
    let path = path.path.trim_start_matches('/');
    let Some(file) = ASSETS.get_file(path) else {
        return StatusCode::NOT_FOUND.into_response();
    };
    let content_type = match path.rsplit_once('.').map(|(_, extension)| extension) {
        Some("css") => "text/css; charset=utf-8",
        Some("js") => "application/javascript; charset=utf-8",
        Some("json") => "application/json",
        Some("svg") => "image/svg+xml",
        Some("ico") => "image/x-icon",
        Some("html") => "text/html; charset=utf-8",
        _ => "application/octet-stream",
    };
    let mut response = bytes_response(StatusCode::OK, content_type, file.contents().to_vec());
    set_header(
        &mut response,
        header::CACHE_CONTROL,
        "public, max-age=31536000, immutable",
    );
    response
}

pub async fn datastar() -> Response {
    immutable(DatastarScript::new().into_response())
}

pub async fn datastar_source_map() -> Response {
    immutable(DatastarSourceMap::new().into_response())
}

fn immutable(mut response: Response) -> Response {
    set_header(
        &mut response,
        header::CACHE_CONTROL,
        "public, max-age=31536000, immutable",
    );
    response
}

pub async fn favicon() -> Response {
    let Some(file) = ASSETS.get_file("favicon.ico") else {
        return StatusCode::NOT_FOUND.into_response();
    };
    let mut response = bytes_response(StatusCode::OK, "image/x-icon", file.contents().to_vec());
    set_header(
        &mut response,
        header::CACHE_CONTROL,
        "public, max-age=31536000, immutable",
    );
    response
}
