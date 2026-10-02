//! ISO 3166-1 alpha-2 lookups for the world-map response.
//!
//! Numeric codes come from the `celes` crate for every alpha-2 code. Names
//! use the same short English names the Go implementation shipped (so the
//! map labels do not change across the port) and fall back to the official
//! `celes` long name for any code that table does not cover.

use celes::Country;

/// Resolved `(numeric_code, english_name)` for an alpha-2 country code.
///
/// Unknown or empty input yields an empty numeric code and the raw input as
/// the name, matching the Go behaviour.
#[must_use]
pub fn resolve(alpha2: &str) -> (String, String) {
    let Some(country) = lookup(alpha2) else {
        return (String::new(), alpha2.to_owned());
    };
    let name =
        short_name(country.alpha2).map_or_else(|| country.long_name.to_owned(), str::to_owned);
    (country.code.to_owned(), name)
}

fn lookup(alpha2: &str) -> Option<Country> {
    if alpha2.len() != 2 || !alpha2.bytes().all(|byte| byte.is_ascii_alphabetic()) {
        return None;
    }
    Country::from_alpha2(alpha2).ok()
}

/// Short English names as used by the Go implementation.
#[allow(clippy::too_many_lines)]
fn short_name(alpha2: &str) -> Option<&'static str> {
    let name = match alpha2 {
        "US" => "United States",
        "CA" => "Canada",
        "MX" => "Mexico",
        "AR" => "Argentina",
        "BR" => "Brazil",
        "CL" => "Chile",
        "CO" => "Colombia",
        "PE" => "Peru",
        "VE" => "Venezuela",
        "EC" => "Ecuador",
        "BO" => "Bolivia",
        "PY" => "Paraguay",
        "UY" => "Uruguay",
        "GB" => "United Kingdom",
        "DE" => "Germany",
        "FR" => "France",
        "ES" => "Spain",
        "IT" => "Italy",
        "NL" => "Netherlands",
        "BE" => "Belgium",
        "CH" => "Switzerland",
        "AT" => "Austria",
        "PT" => "Portugal",
        "IE" => "Ireland",
        "LU" => "Luxembourg",
        "SE" => "Sweden",
        "NO" => "Norway",
        "DK" => "Denmark",
        "FI" => "Finland",
        "IS" => "Iceland",
        "PL" => "Poland",
        "CZ" => "Czechia",
        "SK" => "Slovakia",
        "HU" => "Hungary",
        "RO" => "Romania",
        "BG" => "Bulgaria",
        "UA" => "Ukraine",
        "BY" => "Belarus",
        "RU" => "Russia",
        "MD" => "Moldova",
        "LT" => "Lithuania",
        "LV" => "Latvia",
        "EE" => "Estonia",
        "GR" => "Greece",
        "HR" => "Croatia",
        "SI" => "Slovenia",
        "RS" => "Serbia",
        "BA" => "Bosnia and Herzegovina",
        "ME" => "Montenegro",
        "MK" => "North Macedonia",
        "AL" => "Albania",
        "CY" => "Cyprus",
        "MT" => "Malta",
        "IL" => "Israel",
        "SA" => "Saudi Arabia",
        "AE" => "United Arab Emirates",
        "TR" => "Turkey",
        "IR" => "Iran",
        "IQ" => "Iraq",
        "JO" => "Jordan",
        "LB" => "Lebanon",
        "SY" => "Syria",
        "YE" => "Yemen",
        "OM" => "Oman",
        "KW" => "Kuwait",
        "BH" => "Bahrain",
        "QA" => "Qatar",
        "PS" => "Palestine",
        "CN" => "China",
        "JP" => "Japan",
        "KR" => "South Korea",
        "KP" => "North Korea",
        "TW" => "Taiwan",
        "HK" => "Hong Kong",
        "MO" => "Macau",
        "MN" => "Mongolia",
        "TH" => "Thailand",
        "VN" => "Vietnam",
        "PH" => "Philippines",
        "ID" => "Indonesia",
        "MY" => "Malaysia",
        "SG" => "Singapore",
        "MM" => "Myanmar",
        "KH" => "Cambodia",
        "LA" => "Laos",
        "BN" => "Brunei",
        "TL" => "Timor-Leste",
        "IN" => "India",
        "PK" => "Pakistan",
        "BD" => "Bangladesh",
        "LK" => "Sri Lanka",
        "NP" => "Nepal",
        "AF" => "Afghanistan",
        "BT" => "Bhutan",
        "MV" => "Maldives",
        "KZ" => "Kazakhstan",
        "UZ" => "Uzbekistan",
        "TM" => "Turkmenistan",
        "KG" => "Kyrgyzstan",
        "TJ" => "Tajikistan",
        "EG" => "Egypt",
        "DZ" => "Algeria",
        "MA" => "Morocco",
        "TN" => "Tunisia",
        "LY" => "Libya",
        "SD" => "Sudan",
        "SS" => "South Sudan",
        "NG" => "Nigeria",
        "GH" => "Ghana",
        "CI" => "Côte d'Ivoire",
        "SN" => "Senegal",
        "ML" => "Mali",
        "BF" => "Burkina Faso",
        "NE" => "Niger",
        "GN" => "Guinea",
        "BJ" => "Benin",
        "TG" => "Togo",
        "LR" => "Liberia",
        "SL" => "Sierra Leone",
        "GM" => "Gambia",
        "GW" => "Guinea-Bissau",
        "MR" => "Mauritania",
        "KE" => "Kenya",
        "ET" => "Ethiopia",
        "TZ" => "Tanzania",
        "UG" => "Uganda",
        "SO" => "Somalia",
        "RW" => "Rwanda",
        "BI" => "Burundi",
        "DJ" => "Djibouti",
        "ER" => "Eritrea",
        "CD" => "Democratic Republic of the Congo",
        "CM" => "Cameroon",
        "AO" => "Angola",
        "TD" => "Chad",
        "CF" => "Central African Republic",
        "CG" => "Republic of the Congo",
        "GA" => "Gabon",
        "GQ" => "Equatorial Guinea",
        "ST" => "São Tomé and Príncipe",
        "ZA" => "South Africa",
        "ZW" => "Zimbabwe",
        "ZM" => "Zambia",
        "MW" => "Malawi",
        "MZ" => "Mozambique",
        "BW" => "Botswana",
        "NA" => "Namibia",
        "LS" => "Lesotho",
        "SZ" => "Eswatini",
        "MG" => "Madagascar",
        "MU" => "Mauritius",
        "SC" => "Seychelles",
        "KM" => "Comoros",
        "RE" => "Réunion",
        "AU" => "Australia",
        "NZ" => "New Zealand",
        "PG" => "Papua New Guinea",
        "FJ" => "Fiji",
        "NC" => "New Caledonia",
        "PF" => "French Polynesia",
        "SB" => "Solomon Islands",
        "VU" => "Vanuatu",
        "WS" => "Samoa",
        "GU" => "Guam",
        "AS" => "American Samoa",
        "MP" => "Northern Mariana Islands",
        "FM" => "Micronesia",
        "PW" => "Palau",
        "MH" => "Marshall Islands",
        "KI" => "Kiribati",
        "TO" => "Tonga",
        "TV" => "Tuvalu",
        "NR" => "Nauru",
        "CU" => "Cuba",
        "DO" => "Dominican Republic",
        "HT" => "Haiti",
        "JM" => "Jamaica",
        "TT" => "Trinidad and Tobago",
        "BB" => "Barbados",
        "BS" => "Bahamas",
        "GD" => "Grenada",
        "LC" => "Saint Lucia",
        "VC" => "Saint Vincent and the Grenadines",
        "AG" => "Antigua and Barbuda",
        "DM" => "Dominica",
        "KN" => "Saint Kitts and Nevis",
        "PR" => "Puerto Rico",
        "VI" => "U.S. Virgin Islands",
        "TC" => "Turks and Caicos Islands",
        "KY" => "Cayman Islands",
        "BM" => "Bermuda",
        "AW" => "Aruba",
        "CW" => "Curaçao",
        "GT" => "Guatemala",
        "HN" => "Honduras",
        "SV" => "El Salvador",
        "NI" => "Nicaragua",
        "CR" => "Costa Rica",
        "PA" => "Panama",
        "BZ" => "Belize",
        _ => return None,
    };
    Some(name)
}

#[cfg(test)]
mod tests {
    use super::resolve;

    #[test]
    fn known_countries_resolve_to_numeric_code_and_name() {
        assert_eq!(
            resolve("US"),
            ("840".to_owned(), "United States".to_owned())
        );
        assert_eq!(resolve("FR"), ("250".to_owned(), "France".to_owned()));
        assert_eq!(
            resolve("GB"),
            ("826".to_owned(), "United Kingdom".to_owned())
        );
    }

    #[test]
    fn unknown_or_empty_codes_return_empty_code_and_raw_input() {
        assert_eq!(resolve(""), (String::new(), String::new()));
        assert_eq!(resolve("Unknown"), (String::new(), "Unknown".to_owned()));
        assert_eq!(resolve("XX"), (String::new(), "XX".to_owned()));
        assert_eq!(resolve("1A"), (String::new(), "1A".to_owned()));
    }
}
