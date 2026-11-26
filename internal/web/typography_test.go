// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The tester is a module of the TasmoFleet product family (design doc 4.2), so its type follows
// the TasmoFleet design system (docs/spec/design-system.md, sections "Tokens" and "Type"). The
// rules are written down in docs/design/gui-typography.md; this test holds app.css to them:
//
//  1. The type tokens in :root have the TasmoFleet values. A token that is not in
//     typeTokens is a new size and needs a row there and in gui-typography.md first.
//  2. Every font size outside :root is a var(--fs-*) token, never a literal.
//  3. Font weights are 400, 500 and 600 only.
//  4. Font families are var(--font-sans) or var(--font-mono) only.
//  5. The dark theme does not redefine type tokens.

// typeTokens are the type tokens and their values. Source: TasmoFleet design-system.md.
var typeTokens = map[string]string{
	"--font-sans": `"Geist", ui-sans-serif, system-ui, sans-serif`,
	"--font-mono": `"Geist Mono", ui-monospace, Menlo, monospace`,
	"--fs-base":   "14px",
	"--fs-h1":     "22px",
	"--fs-h2-lg":  "15px",
	"--fs-h2":     "14px",
	"--fs-h2-sm":  "13px",
	"--fs-body":   "13.5px",
	"--fs-ui":     "13px",
	"--fs-meta":   "12.5px",
	"--fs-small":  "12px",
	"--fs-badge":  "11.5px",
	"--lh-body":   "1.45",
	// Planned TasmoFleet tokens (design-system.md "Type", #1100), same values.
	"--fs-title-sm": "16px",
	"--fs-word":     "18px",
	// Tester only: glyph sizes (nav icons, the shield), not text.
	"--fs-icon":    "16px",
	"--fs-icon-lg": "20px",
}

var (
	commentRe   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	fontFaceRe  = regexp.MustCompile(`(?s)@font-face\s*\{.*?\}`)
	declRe      = regexp.MustCompile(`([\w-]+)\s*:\s*([^;{}]+);`)
	tokenRefRe  = regexp.MustCompile(`var\((--(?:fs|lh|font)-[\w-]+)\)`)
	fontSizeRe  = regexp.MustCompile(`^(var\(--fs-[\w-]+\)|inherit)$`)
	fontShortRe = regexp.MustCompile(`^(inherit|(400|500|600) var\(--fs-[\w-]+\)( / ([\d.]+|var\(--lh-[\w-]+\)))? var\(--font-(sans|mono)\))$`)
	fontFamRe   = regexp.MustCompile(`^(var\(--font-(sans|mono)\)|inherit)$`)
	weightRe    = regexp.MustCompile(`^(400|500|600|inherit)$`)
	typeTokenRe = regexp.MustCompile(`^--(fs|lh|font)-`)
)

// ruleBody returns the declarations of the first rule whose selector is sel, and the stylesheet
// with that rule removed.
func ruleBody(t *testing.T, css, sel string) (body, rest string) {
	t.Helper()
	i := strings.Index(css, sel+" {")
	if i < 0 {
		t.Fatalf("no rule %q in app.css", sel)
	}
	j := strings.Index(css[i:], "}")
	if j < 0 {
		t.Fatalf("rule %q is not closed", sel)
	}
	return css[i : i+j], css[:i] + css[i+j+1:]
}

func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func loadCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	return commentRe.ReplaceAllString(string(b), "")
}

func TestTypographyTokensMatchTasmoFleet(t *testing.T) {
	css := loadCSS(t)
	root, _ := ruleBody(t, css, ":root")
	got := map[string]string{}
	for _, d := range declRe.FindAllStringSubmatch(root, -1) {
		if typeTokenRe.MatchString(d[1]) {
			got[d[1]] = normSpace(d[2])
		}
	}
	for name, want := range typeTokens {
		if got[name] != want {
			t.Errorf("%s = %q, want %q (TasmoFleet design-system.md)", name, got[name], want)
		}
	}
	for name := range got {
		if _, ok := typeTokens[name]; !ok {
			t.Errorf("%s is not a known type token: add it to typeTokens and docs/design/gui-typography.md first", name)
		}
	}

	dark, _ := ruleBody(t, css, `[data-theme="dark"]`)
	for _, d := range declRe.FindAllStringSubmatch(dark, -1) {
		if typeTokenRe.MatchString(d[1]) {
			t.Errorf("dark theme redefines %s: type is the same in both themes", d[1])
		}
	}
}

func TestTypographyUsesOnlyTokens(t *testing.T) {
	css := fontFaceRe.ReplaceAllString(loadCSS(t), "")
	_, css = ruleBody(t, css, ":root")

	for _, d := range declRe.FindAllStringSubmatch(css, -1) {
		prop, val := d[1], normSpace(d[2])
		var re *regexp.Regexp
		switch prop {
		case "font-size":
			re = fontSizeRe
		case "font":
			re = fontShortRe
		case "font-family":
			re = fontFamRe
		case "font-weight":
			re = weightRe
		default:
			continue
		}
		if !re.MatchString(val) {
			t.Errorf("%s: %s: use the type tokens (docs/design/gui-typography.md)", prop, val)
		}
		for _, ref := range tokenRefRe.FindAllStringSubmatch(val, -1) {
			if _, ok := typeTokens[ref[1]]; !ok {
				t.Errorf("%s: %s refers to unknown token %s", prop, val, ref[1])
			}
		}
	}
}

func TestPageHasNoInlineStyles(t *testing.T) {
	b, err := os.ReadFile("page.templ")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`\sstyle\s*=`).Match(b) {
		t.Error("page.templ has a style attribute: type and colour come from app.css tokens only")
	}
}
