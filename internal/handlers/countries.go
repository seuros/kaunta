package handlers

import (
	"github.com/biter777/countries"
	"strconv"
)

// getTopoJSONCode converts ISO 3166-1 alpha-2 to TopoJSON numeric codes (ISO 3166-1 numeric)
func getTopoJSONCode(alpha2 string) string {
	// Find country by alpha-2 code
	for _, country := range countries.All() {
		if country.Alpha2() == alpha2 {
			return strconv.Itoa(int(country))
		}
	}
	return ""
}

// getCountryName returns human-readable country names
func getCountryName(alpha2 string) string {
	names := map[string]string{
		// North America
		"US": "United States", "CA": "Canada", "MX": "Mexico",
		// South America
		"AR": "Argentina", "BR": "Brazil", "CL": "Chile", "CO": "Colombia",
		"PE": "Peru", "VE": "Venezuela", "EC": "Ecuador", "BO": "Bolivia",
		"PY": "Paraguay", "UY": "Uruguay",
		// Western Europe
		"GB": "United Kingdom", "DE": "Germany", "FR": "France", "ES": "Spain",
		"IT": "Italy", "NL": "Netherlands", "BE": "Belgium", "CH": "Switzerland",
		"AT": "Austria", "PT": "Portugal", "IE": "Ireland", "LU": "Luxembourg",
		// Northern Europe
		"SE": "Sweden", "NO": "Norway", "DK": "Denmark", "FI": "Finland",
		"IS": "Iceland",
		// Eastern Europe
		"PL": "Poland", "CZ": "Czechia", "SK": "Slovakia", "HU": "Hungary",
		"RO": "Romania", "BG": "Bulgaria", "UA": "Ukraine", "BY": "Belarus",
		"RU": "Russia", "MD": "Moldova", "LT": "Lithuania", "LV": "Latvia",
		"EE": "Estonia",
		// Southern Europe
		"GR": "Greece", "HR": "Croatia", "SI": "Slovenia", "RS": "Serbia",
		"BA": "Bosnia and Herzegovina", "ME": "Montenegro", "MK": "North Macedonia",
		"AL": "Albania", "CY": "Cyprus", "MT": "Malta",
		// Middle East
		"IL": "Israel", "SA": "Saudi Arabia", "AE": "United Arab Emirates",
		"TR": "Turkey", "IR": "Iran", "IQ": "Iraq", "JO": "Jordan",
		"LB": "Lebanon", "SY": "Syria", "YE": "Yemen", "OM": "Oman",
		"KW": "Kuwait", "BH": "Bahrain", "QA": "Qatar", "PS": "Palestine",
		// East Asia
		"CN": "China", "JP": "Japan", "KR": "South Korea", "KP": "North Korea",
		"TW": "Taiwan", "HK": "Hong Kong", "MO": "Macau", "MN": "Mongolia",
		// Southeast Asia
		"TH": "Thailand", "VN": "Vietnam", "PH": "Philippines", "ID": "Indonesia",
		"MY": "Malaysia", "SG": "Singapore", "MM": "Myanmar", "KH": "Cambodia",
		"LA": "Laos", "BN": "Brunei", "TL": "Timor-Leste",
		// South Asia
		"IN": "India", "PK": "Pakistan", "BD": "Bangladesh", "LK": "Sri Lanka",
		"NP": "Nepal", "AF": "Afghanistan", "BT": "Bhutan", "MV": "Maldives",
		// Central Asia
		"KZ": "Kazakhstan", "UZ": "Uzbekistan", "TM": "Turkmenistan",
		"KG": "Kyrgyzstan", "TJ": "Tajikistan",
		// Africa - North
		"EG": "Egypt", "DZ": "Algeria", "MA": "Morocco", "TN": "Tunisia",
		"LY": "Libya", "SD": "Sudan", "SS": "South Sudan",
		// Africa - West
		"NG": "Nigeria", "GH": "Ghana", "CI": "Côte d'Ivoire", "SN": "Senegal",
		"ML": "Mali", "BF": "Burkina Faso", "NE": "Niger", "GN": "Guinea",
		"BJ": "Benin", "TG": "Togo", "LR": "Liberia", "SL": "Sierra Leone",
		"GM": "Gambia", "GW": "Guinea-Bissau", "MR": "Mauritania",
		// Africa - East
		"KE": "Kenya", "ET": "Ethiopia", "TZ": "Tanzania", "UG": "Uganda",
		"SO": "Somalia", "RW": "Rwanda", "BI": "Burundi", "DJ": "Djibouti",
		"ER": "Eritrea",
		// Africa - Central
		"CD": "Democratic Republic of the Congo", "CM": "Cameroon", "AO": "Angola",
		"TD": "Chad", "CF": "Central African Republic", "CG": "Republic of the Congo",
		"GA": "Gabon", "GQ": "Equatorial Guinea", "ST": "São Tomé and Príncipe",
		// Africa - South
		"ZA": "South Africa", "ZW": "Zimbabwe", "ZM": "Zambia", "MW": "Malawi",
		"MZ": "Mozambique", "BW": "Botswana", "NA": "Namibia", "LS": "Lesotho",
		"SZ": "Eswatini", "MG": "Madagascar", "MU": "Mauritius", "SC": "Seychelles",
		"KM": "Comoros", "RE": "Réunion",
		// Oceania
		"AU": "Australia", "NZ": "New Zealand", "PG": "Papua New Guinea",
		"FJ": "Fiji", "NC": "New Caledonia", "PF": "French Polynesia",
		"SB": "Solomon Islands", "VU": "Vanuatu", "WS": "Samoa", "GU": "Guam",
		"AS": "American Samoa", "MP": "Northern Mariana Islands", "FM": "Micronesia",
		"PW": "Palau", "MH": "Marshall Islands", "KI": "Kiribati", "TO": "Tonga",
		"TV": "Tuvalu", "NR": "Nauru",
		// Caribbean
		"CU": "Cuba", "DO": "Dominican Republic", "HT": "Haiti", "JM": "Jamaica",
		"TT": "Trinidad and Tobago", "BB": "Barbados", "BS": "Bahamas",
		"GD": "Grenada", "LC": "Saint Lucia", "VC": "Saint Vincent and the Grenadines",
		"AG": "Antigua and Barbuda", "DM": "Dominica", "KN": "Saint Kitts and Nevis",
		"PR": "Puerto Rico", "VI": "U.S. Virgin Islands", "TC": "Turks and Caicos Islands",
		"KY": "Cayman Islands", "BM": "Bermuda", "AW": "Aruba", "CW": "Curaçao",
		// Central America
		"GT": "Guatemala", "HN": "Honduras", "SV": "El Salvador", "NI": "Nicaragua",
		"CR": "Costa Rica", "PA": "Panama", "BZ": "Belize",
		// Special
		"Unknown": "Unknown",
	}
	if name, ok := names[alpha2]; ok {
		return name
	}
	return alpha2
}
