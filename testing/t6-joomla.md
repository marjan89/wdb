# T6: Joomla — Test Run

**Date**: 2026-04-30
**wdb version**: v0.2.0-dev (33 commands)
**Target**: https://wdb-s-test-123123.joomla.com/administrator/
**Joomla**: 6.1 (CloudAccess.net free tier)
**Credentials**: marjan89@gmail.com / WdbTest2026!

## Scores

| Scenario | Discovery | Targeting | Interaction | Verification | Resilience | Total | Notes |
|----------|-----------|-----------|-------------|--------------|------------|-------|-------|
| S1 Login | 2 | 2 | 2 | 2 | 1 | 9 | Auto-login via CloudAccess. Had to close extra tabs (getPage bug). |
| S2 Map UI | 2 | - | - | - | 2 | 4/4 | `map` captured full sidebar: Content, Menus, Components, Users, System + quick links. |
| S3 Navigate | 2 | 1 | 1 | 2 | 1 | 7 | Sidebar submenus hidden by default. `click --text` didn't navigate. `navigate --link` worked after expanding submenu. Back worked. |
| S4 Read table | 2 | 2 | 2 | 2 | 2 | 10 | `text --all` read article row with all metadata. |
| S5 Create record | 2 | 2 | 1 | 2 | 1 | 8 | Title filled fine. TinyMCE textarea hidden — needed eval for content. Save via eval. Assert confirmed. |
| S8 Delete record | 2 | 2 | 2 | 2 | 2 | 10 | Checkbox + toolbar trash. "Article trashed." confirmed. |

**Score: 48/57 (84%) — Grade: B**

## Issues Found

### 1. Welcome modal + stats consent block everything
Joomla 6.1 shows TWO overlays on first visit: a guided tour modal and a statistics consent banner. Navigation doesn't work until both are dismissed. The tour modal re-appears on page navigation if not permanently dismissed.

**Fix**: Always dismiss overlays before navigating. Add to skill as Joomla-specific gotcha.

### 2. Sidebar submenu visibility
Joomla's sidebar items (Articles, Categories, etc.) are hidden until parent (Content) is clicked. `click --text "Articles"` finds the element but `--force` click doesn't navigate. `navigate --link` works after expanding.

**Pattern**: Same as PrestaShop — sidebar links need `navigate --link` or eval-based navigation.

### 3. TinyMCE hides textarea
Joomla uses TinyMCE which hides the `<textarea>` via `display:none`. `fill` times out. Must use `tinyMCE.activeEditor.setContent()` via eval.

**Fix opportunity**: Add `fill --rich <sel> <html>` that detects TinyMCE/CKEditor and uses their API. Common across WordPress, Joomla, PrestaShop.

### 4. `click --text "Save"` vs "Save & Close"
`click --text "Save"` matched the first "Save" button (Save only), not "Save & Close". Had to use eval to find and click the exact button.

**Already fixed by**: `click --text "Save & Close"` or `click --text "Close" --nth 0`. But `&` in text might cause shell issues.

### 5. `getPage()` returns wrong tab
After tab switch, `getPage()` returns the last tab in the list, not the active one. Had to close extra tabs as workaround.

**Fix opportunity**: Use `browser.MustActivate()` page tracking or match by target activation state.

### 6. Shell `&` in URLs
`navigate "url?a=1&b=2"` — the `&` is eaten by the shell as background operator, even in double quotes via nosandbox. Must use eval for URLs with `&`.

**Fix opportunity**: nosandbox temp script approach should handle this now. Needs verification.

## Optimization Opportunities

### General benefit
1. **`fill --rich`**: Detect TinyMCE/CKEditor/ProseMirror and use their API. Every CMS has a rich text editor. This is a big gap.
2. **`getPage()` active tab fix**: Return the browser's active tab, not the last in list.
3. **Modal auto-dismiss**: Add a `dismiss` command or `--dismiss-modals` flag that closes common overlay patterns (tour, consent, cookie, welcome).

### Site-specific (Joomla)
1. Dismiss tour + stats consent before any interaction.
2. TinyMCE content requires `tinyMCE.activeEditor.setContent()`.
3. Toolbar buttons: "Save & Close" is a dropdown item, not a direct button. Use eval to click specific toolbar actions.
4. Sidebar requires expanding parent menu before clicking submenu items.
