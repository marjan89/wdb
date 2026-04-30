# T2: PrestaShop — Test Run

**Date**: 2026-04-30
**wdb version**: v0.2.0-dev (33 commands)
**Target**: https://demo.prestashop.com → careful-request.demo.prestashop.com
**PrestaShop**: 8.x (Symfony)

## Scores

| Scenario | Discovery | Targeting | Interaction | Verification | Resilience | Total | Notes |
|----------|-----------|-----------|-------------|--------------|------------|-------|-------|
| S1 Login | 2 | 2 | 2 | 2 | 1 | 9 | Pre-filled creds in iframe 0. Login worked. Had to probe frame structure. |
| S2 Map UI | 2 | - | - | - | 2 | 4/4 | `map` captured entire nav-sidebar: 15 top-level sections, all submenus. |
| S3 Navigate | 1 | 1 | 1 | 2 | 1 | 6 | Sidebar clicks don't navigate — PS uses tokenized URLs. Had to use eval to extract href and navigate. |
| S4 Read table | 2 | 2 | 2 | 2 | 2 | 10 | `text --all "table tbody tr"` got 7 products with all columns. |
| S7 Edit record | 1 | 2 | 2 | 2 | 1 | 8 | Needed eval to navigate to edit page. Once there, fill worked perfectly. |
| S9 Search | 2 | 2 | 2 | 2 | 1 | 9 | Header search worked, found 5 results. Title confirmed "Search results". |

**Score: 46/57 (81%) — Grade: B**

## Issues Found

### 1. Demo iframe wrapper hides real URLs
PrestaShop demo wraps the back office in an iframe. All frames report the same URL (`demo.prestashop.com/#/en/back`), making URL-based debugging impossible. Must work within frame 0.

### 2. Sidebar navigation doesn't work with click
PrestaShop admin uses tokenized URLs (CSRF `_token` parameter). Clicking sidebar links via rod's MustClick or JS click doesn't trigger the SPA-style navigation. The links have hrefs but clicking them doesn't navigate.

**Root cause**: The sidebar might use JavaScript event handlers that prevent default link behavior, or the navigation is handled by a Symfony JavaScript router.

**Workaround**: Use eval to extract the href from the sidebar link and navigate via `window.location.href = link.href`.

**Fix opportunity**: Add a `navigate --link <sel>` command that extracts an element's href and navigates to it. This is a common pattern: "click this link to navigate" but the link uses JS interception.

### 3. `click --text` ambiguity on product names
`click --text "Customizable mug"` failed with "no visible shape" because the element was below the viewport. Even after scrolling, `--force` clicked but didn't navigate because the link uses JS event handling.

**Workaround**: eval-based navigation.

### 4. `--within` timeout on complex DOM
`click --text "Products" --within "#nav-sidebar"` timed out with "context deadline exceeded". The `ElementR("*", regex)` search within a container with hundreds of elements is too slow.

**Fix opportunity**: Use a smarter search strategy in `--within` — search `a, button` instead of `*`.

## Optimization Opportunities

### General benefit
1. **`--within` performance**: Change `ElementR("*", regex)` to `ElementR("a, button, [role=link], [role=button]", regex)` to avoid searching every DOM node.
2. **`navigate --link <sel>`**: Extract href from an element and navigate to it. Handles SPA frameworks that intercept clicks.

### Site-specific (PrestaShop)
1. PrestaShop uses CSRF-tokenized URLs — sidebar links can't be pre-computed. Must extract dynamically.
2. Demo wrapper adds iframe layer. Real installs won't have this issue.
3. Product edit forms have multilingual field selectors like `#product_header_name_6` (6 = language ID). These are dynamic.

## Scenarios Not Run
- S5 (create record) — demo is read-only for product creation
- S6 (complex form) — product variant generator, would need writable instance
- S8 (delete) — demo likely restricts deletion
- S10 (modal) — not tested
- S12 (keyboard) — not tested
- S13 (error handling) — not tested
- S14 (multi-step workflow) — not tested
