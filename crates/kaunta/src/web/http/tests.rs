use std::net::{IpAddr, Ipv4Addr};

use crate::domain::website::ProxyMode;
use rama::{
    extensions::ExtensionsRef as _,
    http::{Body, Request},
    net::{address::SocketAddress, stream::SocketInfo},
};

use super::{client_ip, config_origin_trusted, effective_proxy_mode};

const PEER: IpAddr = IpAddr::V4(Ipv4Addr::new(203, 0, 113, 9));

fn build_request(headers: &[(&'static str, &str)]) -> Request {
    let mut builder = Request::builder();
    for (name, value) in headers {
        builder = builder.header(*name, *value);
    }
    let request = builder.body(Body::empty()).expect("valid request");
    request
        .extensions()
        .insert(SocketInfo::new(None, SocketAddress::new(PEER, 4321)));
    request
}

#[test]
fn none_mode_ignores_forwarding_headers() {
    let request = build_request(&[
        ("x-forwarded-for", "198.51.100.1, 10.0.0.1"),
        ("x-real-ip", "198.51.100.2"),
        ("cf-connecting-ip", "198.51.100.3"),
    ]);
    assert_eq!(
        client_ip(&request, ProxyMode::None, ProxyMode::None),
        PEER.to_string()
    );
}

#[test]
fn xforwarded_uses_first_chain_element() {
    let request = build_request(&[("x-forwarded-for", " 198.51.100.1 , 10.0.0.1")]);
    assert_eq!(
        client_ip(&request, ProxyMode::Xforwarded, ProxyMode::None),
        "198.51.100.1"
    );
}

#[test]
fn xforwarded_falls_back_to_real_ip_then_peer() {
    let request = build_request(&[("x-real-ip", "198.51.100.2")]);
    assert_eq!(
        client_ip(&request, ProxyMode::Xforwarded, ProxyMode::None),
        "198.51.100.2"
    );
    let request = build_request(&[("x-forwarded-for", " , ")]);
    assert_eq!(
        client_ip(&request, ProxyMode::Xforwarded, ProxyMode::None),
        PEER.to_string()
    );
}

#[test]
fn cloudflare_prefers_first_cf_part_then_forwarded_chain() {
    let request = build_request(&[
        ("cf-connecting-ip", "198.51.100.3, 10.0.0.2"),
        ("x-forwarded-for", "198.51.100.1"),
    ]);
    assert_eq!(
        client_ip(&request, ProxyMode::Cloudflare, ProxyMode::None),
        "198.51.100.3"
    );
    let request = build_request(&[("x-real-ip", "198.51.100.2")]);
    assert_eq!(
        client_ip(&request, ProxyMode::Cloudflare, ProxyMode::None),
        "198.51.100.2"
    );
    let request = build_request(&[]);
    assert_eq!(
        client_ip(&request, ProxyMode::Cloudflare, ProxyMode::None),
        PEER.to_string()
    );
}

#[test]
fn website_mode_overrides_server_mode_unless_none() {
    assert_eq!(
        effective_proxy_mode(ProxyMode::None, ProxyMode::Cloudflare),
        ProxyMode::Cloudflare
    );
    assert_eq!(
        effective_proxy_mode(ProxyMode::Xforwarded, ProxyMode::Cloudflare),
        ProxyMode::Xforwarded
    );
    assert_eq!(
        effective_proxy_mode(ProxyMode::None, ProxyMode::None),
        ProxyMode::None
    );

    let request = build_request(&[
        ("cf-connecting-ip", "198.51.100.3"),
        ("x-forwarded-for", "198.51.100.1"),
    ]);
    assert_eq!(
        client_ip(&request, ProxyMode::None, ProxyMode::Xforwarded),
        "198.51.100.1"
    );
    assert_eq!(
        client_ip(&request, ProxyMode::Cloudflare, ProxyMode::Xforwarded),
        "198.51.100.3"
    );
}

#[test]
fn missing_peer_falls_back_to_loopback() {
    let request = Request::builder()
        .body(Body::empty())
        .expect("valid request");
    assert_eq!(
        client_ip(&request, ProxyMode::None, ProxyMode::None),
        "127.0.0.1"
    );
}

#[test]
fn config_origin_matching_normalizes_scheme_and_case() {
    let trusted = vec!["Analytics.Example.com/".to_owned()];
    assert!(config_origin_trusted(
        &trusted,
        "https://analytics.example.com"
    ));
    assert!(config_origin_trusted(
        &trusted,
        "http://analytics.example.com/"
    ));
    assert!(!config_origin_trusted(&trusted, "https://attacker.example"));
    assert!(!config_origin_trusted(&[], "https://analytics.example.com"));
}
