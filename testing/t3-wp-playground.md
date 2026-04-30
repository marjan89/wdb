# T3: WP Playground — Test Run

**Date**: 2026-04-30
**wdb version**: v0.2.0-dev (33 commands)
**Target**: https://playground.wordpress.net
**WordPress**: 6.9.4, Twenty Twenty-Five theme

## Scores

| Scenario | Discovery | Targeting | Interaction | Verification | Resilience | Total | Notes |
|----------|-----------|-----------|-------------|--------------|------------|-------|-------|
| S1 Login | 2 | 2 | 2 | 2 | 2 | 10 | Auto-login, frame 0.0, assert confirmed dashboard |
| S2 Map UI | 2 | - | - | - | 2 | 4/4 | `map` captured full sidebar + toolbar hierarchy |
| S3 Navigate | 2 | 2 | 2 | 2 | 2 | 10 | 3 sections + back, all verified via assert |
| S4 Read table | 2 | 2 | 2 | 2 | 2 | 10 | `text "#the-list"` got all 3 pages with metadata |
| S5 Create record | 2 | 2 | 2 | 2 | 2 | 10 | Quick Draft: fill title+content, save, verified in Recent Drafts |
| S7 Edit record | 2 | 2 | 2 | 2 | 2 | 10 | Changed site title+tagline, saved, assert confirmed notice + value |
| S8 Delete record | 1 | 1 | 1 | 2 | 1 | 6 | Trash link hidden by CSS, click --text hit wrong element, needed eval fallback |
| S9 Search | 2 | 2 | 2 | 2 | 2 | 10 | Search found draft, text confirmed results |
| S10 Modal | 2 | 2 | 2 | 2 | 2 | 10 | Screen Options opened, text read contents, closed |
| S12 Keyboard | 2 | 2 | 2 | 2 | 2 | 10 | Tab moved focus through form fields, eval confirmed active element |
| S13 Error handling | 1 | 2 | 2 | 1 | 1 | 7 | Error notice not in standard selector, needed eval to find it |

**Score: 97/107 (91%) — Grade: A**

## Issues Found

### 1. `click --text` ambiguity (S3, S8)
`click --text "Posts"` matched a non-sidebar element on the dashboard. `click --text "Trash"` matched the bulk action option instead of the row action. Text matching is greedy — first match wins.

**Workaround**: Use CSS selectors (`#menu-posts a.menu-top`) instead of text matching when multiple elements share the same label.

**Fix opportunity**: Add `click --text "X" --within "selector"` to scope text search to a container.

### 2. CSS-hidden row actions (S8)
WP admin hides row actions (`Edit | Quick Edit | Trash`) via CSS until row hover. `hover` on the post title triggers the CSS `:hover` but rod's `MustClick` still sees the element as having "no visible shape" because visibility transitions may not complete synchronously.

**Workaround**: `eval '() => { document.querySelector(".submitdelete").click(); return "done"; }'`

**Fix opportunity**: Add a `click --force <sel>` that calls `el.MustEval("() => this.click()")` — JS click ignores visibility. Common pattern in Playwright (`{force: true}`).

### 3. Error notice selectors (S13)
WP error notices use inconsistent selectors: `#message`, `.notice`, `.error`, `[role=alert]`, `.notice-error`. The error after creating a user with bad email appeared in `#message.notice.notice-error` but none of the simple selectors matched within timeout because the page had reloaded.

**Workaround**: Use eval to search multiple selectors at once.

**Fix opportunity**: No wdb change needed — this is a project-level nav file issue. Document WP's notice selectors in the navigation file.

### 4. WP Playground iframe navigation (S1)
Must use `frame 0.0` then `eval '() => { window.location.href = "/wp-admin/"; }'` to get into the admin. Cannot use `navigate` because it navigates the outer page. Cannot use `click --text "About WordPress"` because admin bar items are outside viewport in the iframe.

**Workaround**: eval-based navigation within the iframe works reliably.

**Fix opportunity**: None — this is WP Playground's architecture (service worker + nested iframes), not a wdb limitation.

## Optimization Opportunities

### General benefit (applicable to all sites)
1. **`click --force`**: JS-based click that ignores visibility. Useful for CSS-hidden elements (row actions, hover menus). Every admin panel has some form of hidden-until-hover actions.
2. **`click --text --within`**: Scope text matching to a container element. Prevents ambiguous matches when the same text appears in sidebar, content, and toolbar.
3. **`text --all <sel>`**: Return text from ALL matching elements, not just the first. Useful for reading table rows, list items, error messages.

### Site-specific (WP only)
1. WP error notices use many selectors — document in nav file, not in wdb.
2. WP Playground requires `frame 0.0` + eval navigation — unavoidable due to service worker architecture.
3. Quick Draft widget is a good simple-form test; Gutenberg editor is needed for S6 (complex form).

## Scenarios Not Run
- S6 (complex form) — needs Gutenberg editor, which is a full React app
- S11 (context menu) — WP admin doesn't use custom context menus
- S14 (multi-step workflow) — would need order/woocommerce plugin
