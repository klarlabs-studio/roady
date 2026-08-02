package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// slugMaxLen bounds a generated id. Ids reach filenames and path components,
// where an unbounded name is a portability problem on several filesystems.
const slugMaxLen = 64

// slugSeparator joins the words of an id.
const slugSeparator = '-'

// Slugify derives an identifier from a human-written title.
//
// Ids are not merely cosmetic here: a feature id becomes a path component
// under .roady/projects/<name>/, a URL segment in exported views, and an
// argument the user types into a shell. A naive slugifier that only replaces
// spaces lets through characters that break all three — '/' silently nests a
// path, '+' decodes back to a space in a query string, and '(' opens a
// subshell — so a title like "Wearable Ingestion (Garmin / Polar)" produced an
// id that could not be pasted into a command without quoting.
//
// Everything outside letters and digits therefore collapses to a single
// separator. Latin letters carrying diacritics fold to ASCII so that
// "Prüfung" and "Prufung" do not become two different ids; letters from
// scripts with no ASCII equivalent are kept as they are, since dropping them
// would leave nothing behind.
func Slugify(title string) string {
	var b strings.Builder
	b.Grow(len(title))

	pendingSeparator := false
	for _, r := range title {
		if folded, ok := foldLatin(r); ok {
			if pendingSeparator && b.Len() > 0 {
				b.WriteRune(slugSeparator)
			}
			pendingSeparator = false
			b.WriteString(folded)
			continue
		}

		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSeparator && b.Len() > 0 {
				b.WriteRune(slugSeparator)
			}
			pendingSeparator = false
			b.WriteRune(unicode.ToLower(r))
			continue
		}

		// Anything else — punctuation, symbols, whitespace, control
		// characters — is a word boundary rather than content. Recording the
		// boundary instead of emitting it collapses runs like " -- " to a
		// single separator and drops leading and trailing ones entirely.
		pendingSeparator = true
	}

	slug := truncateSlug(b.String())
	if slug == "" {
		// A title of pure punctuation still needs a usable id; an empty one
		// would collide with every other such feature. Derive it from the
		// title so it is stable across runs and distinct between titles.
		return fallbackSlug(title)
	}
	return slug
}

// truncateSlug bounds the id without leaving a dangling separator, preferring
// to cut at a word boundary so the result stays readable.
func truncateSlug(slug string) string {
	if len(slug) <= slugMaxLen {
		return slug
	}

	slug = slug[:slugMaxLen]
	if idx := strings.LastIndexByte(slug, slugSeparator); idx > 0 {
		slug = slug[:idx]
	}
	return strings.Trim(slug, string(slugSeparator))
}

// fallbackSlug names a title that contained no letters or digits at all.
func fallbackSlug(title string) string {
	sum := sha256.Sum256([]byte(title))
	return "id-" + hex.EncodeToString(sum[:4])
}

// foldLatin maps a Latin letter carrying a diacritic to its ASCII form,
// returning false for runes it does not cover. Only the ranges that appear in
// Western European prose are handled; anything else falls through to being
// kept verbatim.
func foldLatin(r rune) (string, bool) {
	if folded, ok := latinFolding[unicode.ToLower(r)]; ok {
		return folded, true
	}
	return "", false
}

var latinFolding = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'ç': "c", 'ć': "c", 'č': "c", 'ĉ': "c", 'ċ': "c",
	'ð': "d", 'ď': "d", 'đ': "d",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ĕ': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'ĝ': "g", 'ğ': "g", 'ġ': "g", 'ģ': "g",
	'ĥ': "h", 'ħ': "h",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ĩ': "i", 'ī': "i", 'ĭ': "i", 'į': "i", 'ı': "i",
	'ĵ': "j",
	'ķ': "k",
	'ĺ': "l", 'ļ': "l", 'ľ': "l", 'ł': "l",
	'ñ': "n", 'ń': "n", 'ņ': "n", 'ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ŏ': "o", 'ő': "o",
	'ŕ': "r", 'ŗ': "r", 'ř': "r",
	'ś': "s", 'ŝ': "s", 'ş': "s", 'š': "s",
	'ţ': "t", 'ť': "t", 'ŧ': "t",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ũ': "u", 'ū': "u", 'ŭ': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'ŵ': "w",
	'ý': "y", 'ÿ': "y", 'ŷ': "y",
	'ź': "z", 'ż': "z", 'ž': "z",

	// Ligatures and letters that expand to more than one ASCII character.
	'ß': "ss", 'æ': "ae", 'œ': "oe", 'þ': "th",
}
