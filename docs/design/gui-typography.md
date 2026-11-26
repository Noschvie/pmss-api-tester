# GUI Typography

The PMSS API Tester is a module of the TasmoFleet product family (design document, section 4.2).
Its type therefore follows the TasmoFleet design system
([`docs/spec/design-system.md`](https://github.com/hellwand/tasmofleet/blob/main/docs/spec/design-system.md),
sections "Tokens" and "Type"). This page records which parts are adopted and how they are enforced.

## Rules

1. Body text is `400 14px/1.45` in Geist (`--fs-base`, `--lh-body`, `--font-sans`).
2. Every font size is a `--fs-*` token from the table below. A size that is not in the table is not
   used; a new one needs a row here and in `typeTokens` (`internal/web/typography_test.go`) first.
3. Font weights are 400, 500 and 600 only.
4. Font families are `var(--font-sans)` or `var(--font-mono)` only; Geist and Geist Mono are
   self-hosted (`internal/web/static/geist-*.woff2`).
5. A component writes `font: <weight> var(--fs-x)[ / <line-height>] var(--font-sans|--font-mono)`
   or sets `font-size` / `font-weight` with tokens.
6. Type is the same in the light and dark theme; `[data-theme="dark"]` redefines colours only.
7. No inline styles in templates.

## Tokens

| Token | Value | Use in the tester |
|---|---|---|
| `--fs-h1` | 22px | page title (`600`, line-height 1.2, letter-spacing -0.015em) |
| `--fs-word` | 18px | a word as a status card value ("Connected") |
| `--fs-title-sm` | 16px | welcome card heading |
| `--fs-h2-lg` | 15px | section headings (Operations, Result) |
| `--fs-h2` | 14px | card and panel headings, operation titles |
| `--fs-base` | 14px | body |
| `--fs-body` | 13.5px | subtitle, navigation, inputs, check titles |
| `--fs-h2-sm` | 13px | headings inside small cards |
| `--fs-ui` | 13px | buttons |
| `--fs-meta` | 12.5px | field labels, secondary lines, crumbs, mono values (paths, URLs, headers, schemas) |
| `--fs-small` | 12px | hints, field help, footers, counts |
| `--fs-badge` | 11.5px | badges (methods, check status), eyebrow and nav captions |
| `--fs-icon` | 16px | tester only: glyphs (brand mark, nav icons) |
| `--fs-icon-lg` | 20px | tester only: the shield glyph |

`--fs-title-sm` and `--fs-word` are planned TasmoFleet tokens (design-system.md, "Type", #1100)
with the same values. `--fs-stat` (28px) is not used: the tester shows no numeric tiles.

The smallest text is 11.5px (badges); hints and help are 12px.

## Enforcement

`internal/web/typography_test.go` fails when

- a type token in `:root` differs from the table above or is not listed in it,
- a `font-size`, `font`, `font-weight` or `font-family` declaration uses a literal value instead of
  the tokens, or a weight other than 400/500/600,
- the dark theme redefines a type token,
- `page.templ` contains a `style` attribute.

Visual regression baselines (`e2e/visual.spec.js`) are taken after this check passes.
