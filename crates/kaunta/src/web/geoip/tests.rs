use super::{DATABASE_FILE, GeoIp, database_path};
use rama::geo::Country;
use rama::net::address::ip::geo::{
    GeoLocation, IpGeoDb, IpVersion, MmdbBuilder, MmdbReader, Subdivision,
};
use std::path::Path;
use std::sync::Arc;

fn reader_for(cidr: &str, location: &GeoLocation) -> MmdbReader {
    let mut builder = MmdbBuilder::new(IpVersion::V4, "Kaunta-Test");
    builder.insert(cidr.parse().unwrap(), location).unwrap();
    MmdbReader::from_bytes(builder.build().unwrap()).unwrap()
}

#[test]
fn database_lives_in_data_directory() {
    assert_eq!(
        database_path(Path::new("/var/lib/kaunta")),
        Path::new("/var/lib/kaunta").join(DATABASE_FILE)
    );
}

#[test]
fn disabled_lookup_is_empty() {
    assert_eq!(
        GeoIp::default().lookup("203.0.113.1"),
        (String::new(), String::new(), String::new())
    );
}

#[test]
fn rama_db_preserves_country_city_and_first_subdivision() {
    let location = GeoLocation {
        country: Some(Country::Morocco),
        city: Some("Casablanca".into()),
        subdivisions: vec![Subdivision {
            iso_code: Some("06".into()),
            name: Some("Casablanca-Settat".into()),
        }],
        ..GeoLocation::default()
    };
    let db = IpGeoDb::builder()
        .reader("kaunta-test", reader_for("203.0.113.0/24", &location))
        .build();
    let geoip = GeoIp {
        db: Some(Arc::new(db)),
    };
    assert!(geoip.is_enabled());
    assert_eq!(
        geoip.lookup("203.0.113.1"),
        (
            "MA".to_owned(),
            "Casablanca".to_owned(),
            "Casablanca-Settat".to_owned(),
        )
    );
    assert_eq!(geoip.lookup("not-an-ip"), super::empty_location());
    assert_eq!(geoip.lookup("192.0.2.1"), super::empty_location());
    assert_eq!(
        geoip.lookup("::ffff:203.0.113.1"),
        geoip.lookup("203.0.113.1")
    );
}

#[test]
fn multiple_sources_merge_in_order() {
    let country_only = GeoLocation {
        country: Some(Country::Morocco),
        ..GeoLocation::default()
    };
    let city_only = GeoLocation {
        city: Some("Casablanca".into()),
        ..GeoLocation::default()
    };
    let db = IpGeoDb::builder()
        .reader("country-db", reader_for("203.0.113.0/24", &country_only))
        .reader("city-db", reader_for("203.0.113.0/24", &city_only))
        .build();
    let geoip = GeoIp {
        db: Some(Arc::new(db)),
    };
    assert_eq!(
        geoip.lookup("203.0.113.1"),
        ("MA".to_owned(), "Casablanca".to_owned(), String::new())
    );
}
