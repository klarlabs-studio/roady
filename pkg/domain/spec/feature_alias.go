package spec

import "strings"

// FeatureAliases maps every spelling of a feature roady accepts in a task's
// feature_id — the id, the title, and the slug of each, normalised so case
// and spacing do not matter — to the feature's id. The first feature to claim
// a spelling keeps it, so an ambiguous title cannot take another feature's
// tasks.
type FeatureAliases struct {
	ids     map[string]bool
	byAlias map[string]string
}

// NewFeatureAliases indexes features for resolving feature_id spellings.
func NewFeatureAliases(features []Feature) *FeatureAliases {
	a := &FeatureAliases{ids: make(map[string]bool, len(features)), byAlias: make(map[string]string, len(features)*3)}
	for _, f := range features {
		if f.ID == "" {
			continue
		}
		a.ids[f.ID] = true
		for _, alias := range []string{f.ID, f.Title, Slugify(f.Title), Slugify(f.ID)} {
			if key := normaliseAlias(alias); key != "" {
				if _, taken := a.byAlias[key]; !taken {
					a.byAlias[key] = f.ID
				}
			}
		}
	}
	return a
}

// IsID reports whether name is a feature id as written.
func (a *FeatureAliases) IsID(name string) bool { return a.ids[name] }

// Resolve returns the id of the feature name spells, and whether one does.
func (a *FeatureAliases) Resolve(name string) (string, bool) {
	if a.ids[name] {
		return name, true
	}
	id, ok := a.byAlias[normaliseAlias(name)]
	return id, ok
}

// normaliseAlias reduces a spelling to the form used for matching. It is
// deliberately more forgiving than Slugify: matching is a lookup, not an id.
func normaliseAlias(s string) string {
	return Slugify(strings.TrimSpace(strings.ToLower(s)))
}
