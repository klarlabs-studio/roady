package audit

import "strconv"

// plural prefixes a count and picks the matching verb phrase.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + pluralForm
}
