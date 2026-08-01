package report

import "strconv"

// pct renders a percentage without trailing noise: 42.0 -> "42%", 42.5 -> "42.5%".
func pct(v float64) string {
	if v == float64(int(v)) {
		return strconv.Itoa(int(v)) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

// plural picks the singular or plural noun for n and prefixes the count.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + pluralForm
}
