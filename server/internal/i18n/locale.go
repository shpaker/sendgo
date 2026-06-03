// Package i18n implements minimal server-side locale negotiation for the SSR
// `<html lang>` attribute.
//
// The current Node server uses @fluent/* to populate the lang attribute and
// title/description when rendering dist/index.html. We only replicate the lang
// part — title/description translation is handled by client-side JS (Fluent on
// the frontend).
package i18n

import (
	"strings"

	"golang.org/x/text/language"
)

// Negotiator picks a locale from the available list based on Accept-Language.
type Negotiator struct {
	matcher    language.Matcher
	available  []language.Tag
	avail2name map[language.Tag]string
}

// NewNegotiator takes the list of locale codes (as they appear in
// public/locales/, e.g. "en-US", "ru", "de-DE") and builds a matcher. The
// first code is used as the fallback default.
func NewNegotiator(availableNames []string) *Negotiator {
	if len(availableNames) == 0 {
		availableNames = []string{"en-US"}
	}
	tags := make([]language.Tag, 0, len(availableNames))
	avail2name := make(map[language.Tag]string, len(availableNames))
	for _, name := range availableNames {
		t := language.Make(name)
		tags = append(tags, t)
		avail2name[t] = name
	}
	return &Negotiator{
		matcher:    language.NewMatcher(tags),
		available:  tags,
		avail2name: avail2name,
	}
}

// Negotiate returns the name of the chosen locale (as it appears in the
// available list). Empty or invalid Accept-Language returns the first
// (default) one.
func (n *Negotiator) Negotiate(acceptLanguage string) string {
	if strings.TrimSpace(acceptLanguage) == "" {
		return n.avail2name[n.available[0]]
	}
	// language.MatchStrings silently ignores per-tag parse errors.
	tag, _, _ := n.matcher.Match(parseAcceptLanguage(acceptLanguage)...)
	if name, ok := n.avail2name[tag]; ok {
		return name
	}
	// The matcher may return a base tag that does not strictly match anything
	// in available. Fall back to nearest match by base.
	base, _ := tag.Base()
	for t, name := range n.avail2name {
		b, _ := t.Base()
		if b == base {
			return name
		}
	}
	return n.avail2name[n.available[0]]
}

func parseAcceptLanguage(h string) []language.Tag {
	parts := strings.Split(h, ",")
	tags := make([]language.Tag, 0, len(parts))
	for _, p := range parts {
		// Strip the quality factor (e.g. "en;q=0.8").
		if i := strings.Index(p, ";"); i >= 0 {
			p = p[:i]
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		t, err := language.Parse(p)
		if err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}
