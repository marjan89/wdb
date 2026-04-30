# Web Control — Capability Tracker & Test Plan

## Current Capabilities (v0.1.0)

### Core Interaction Loop

| Capability | Status | Notes |
|-----------|--------|-------|
| Launch browser with CDP | DONE | Chrome/Chromium via `--remote-debugging-port`, `--no-sandbox` for claude-safe |
| Connect to running browser | DONE | `connect_over_cdp` on 127.0.0.1, configurable port |
| Compact UI dump (`ui`) | DONE | JS extraction of interactive elements, `*`/`~` markers, coords + selector hints |
| Raw accessibility tree (`ui --raw`) | DONE | `page.accessibility.snapshot()` JSON output |
| JSON element dump (`ui --json`) | DONE | Full element data for programmatic use |
| Off-screen element detection | DONE | `--all` flag, viewport filtering, count of hidden elements |
| Screenshot | DONE | Auto-downscale via `sips -Z 1200` |
| Click by CSS selector | DONE | `page.click()` with timeout |
| Click by visible text | DONE | `get_by_text().first.click()` |
| Click by ARIA role + name | DONE | `get_by_role().first.click()` |
| Click by coordinates | DONE | `page.mouse.click(x, y)` — fallback only |
| Type into focused element | DONE | `page.keyboard.type()` |
| Fill (clear + type) by selector | DONE | `page.fill()` |
| Select dropdown option | DONE | `page.select_option()` |
| Press keyboard key | DONE | `page.keyboard.press()` — Enter, Tab, Escape, shortcuts |
| Navigate to URL | DONE | `page.goto()` with `domcontentloaded` wait |
| Back / Forward / Reload | DONE | Standard browser navigation |
| Scroll (up/down/top/bottom) | DONE | `mouse.wheel()` or Home/End keys |
| Tab management (list/switch/close) | DONE | CDP multi-page support |
| Hover | DONE | `page.hover()` |
| Wait for selector | DONE | `page.wait_for_selector()` with configurable timeout |
| Eval JavaScript | DONE | `page.evaluate()` — arbitrary JS execution |
| Browser lifecycle (launch/stop) | DONE | PID tracking, process management |
| Shadow DOM walking | DONE | `ui` JS walks `el.shadowRoot` recursively |

### UI Mapping (`map` command)

| Capability | Status | Notes |
|-----------|--------|-------|
| Menubar discovery | DONE | Finds `[role="menubar"]` items, falls back to `button[aria-haspopup]` |
| Menu dropdown extraction | DONE | Clicks each menu, extracts visible `[role="menuitem"]` elements |
| Submenu expansion (1 level) | DONE | Hover on items with `aria-haspopup` or `►`, extract child items |
| Keyboard shortcut extraction | DONE | Parses last span for `⌘/⌥/⇧/Ctrl` patterns + `aria-keyshortcuts` |
| Disabled/checked state | DONE | `aria-disabled`, `aria-checked` detection |
| Toolbar button cataloging | DONE | Finds `[role="toolbar"]` children with labels and IDs |
| Context menu (right-click) | DONE | Right-clicks content area, captures menu items |
| Navigation file output | DONE | Structured markdown with tables |

### Page selection / connection

| Capability | Status | Notes |
|-----------|--------|-------|
| Skip `chrome://` internal pages | DONE | `get_page()` filters out internal URLs |
| Skip `about:blank` | DONE | Prefers pages with real content |
| IPv4 connection | DONE | Uses `127.0.0.1` not `localhost` (avoids IPv6 issues) |

---

## Needed Capabilities (Gaps)

### Priority 1 — Required for basic user journey replay

| Gap | Why it matters | Difficulty |
|-----|---------------|------------|
| **Multi-level submenu recursion** | Google Sheets Insert > Function > Math > ... goes 3+ deep. Current map only does 1 level. | Medium |
| **iframe content inspection** | Many admin panels (TinyMCE, CKEditor, embedded widgets) use iframes. `ui` only sees main frame. | Medium |
| **File upload handling** | Product forms need image uploads. Playwright has `set_input_files()` but no `web.py` command. | Easy |
| **Waiting for AJAX/SPA transitions** | After clicking "Save" in Shopify/PrestaShop, content reloads via AJAX. Need smart wait-for-change. | Medium |
| **Form state readback** | After filling a form, verify all fields have expected values. Current `ui` shows interactive elements but not all form field values comprehensively. | Medium |

### Priority 2 — Required for complex admin panels

| Gap | Why it matters | Difficulty |
|-----|---------------|------------|
| **Sidebar/panel mapping** | Google Sheets Tables sidebar, Odoo chatter panels, WP Customizer — `map` misses non-menu panels. | Medium |
| **Modal/dialog detection** | Format > Conditional Formatting opens a modal. Need to detect overlays and map their contents. | Medium |
| **Drag-and-drop** | WP Gutenberg blocks, Odoo kanban cards, sortable lists. Playwright supports it but no `web.py` command. | Medium |
| **Rich text editor interaction** | TinyMCE, CKEditor, ProseMirror, Tiptap — contenteditable divs need special handling (not `fill`). | Hard |
| **Autocomplete/typeahead fields** | Odoo many2one fields, jQuery UI autocomplete — type then select from dropdown suggestions. | Medium |
| **Date/time picker interaction** | Custom date pickers (not native `<input type="date">`) require clicking calendar widgets. | Medium |
| **Multi-select / tag inputs** | Joomla tags, WooCommerce product tags — type-to-search, click to add, chips to remove. | Medium |

### Priority 3 — Required for reliable production use

| Gap | Why it matters | Difficulty |
|-----|---------------|------------|
| **Route/page crawling** | Walk all same-origin links, map each page's UI. For multi-page apps (Joomla, WP admin). | Medium |
| **State-dependent UI mapping** | Elements that appear only after selecting a row, hovering, or toggling a mode. | Hard |
| **Cookie/auth persistence docs** | Document how sessions survive across `stop`/`launch`. Handle session expiry gracefully. | Easy |
| **Assertion helpers** | `web.py assert <selector> <text>` — verify element contains expected text. For test scripts. | Easy |
| **Batch command / script mode** | Run a sequence of commands from a file: `web.py run scenario.txt`. For repeatable journeys. | Medium |
| **Network request interception** | Wait for specific XHR/fetch to complete before proceeding. Critical for AJAX-heavy apps. | Hard |
| **Error recovery / retry** | If a click misses (element not yet visible), retry with backoff instead of failing. | Medium |

---

## Test Targets

### T1: OpenCart Admin

- **URL**: https://demo.opencart.com/admin/
- **Credentials**: `demo` / `demo` (pre-filled)
- **Stack**: PHP, MySQL, custom MVC
- **Resets**: Periodically (read-only safe)
- **UI complexity**: Multi-tab product forms, rich text editor (CKEditor/Summernote), image manager modal, autocomplete inputs, data tables with pagination, nested category tree, order status workflows
- **Key challenge**: Tab-based forms where each tab is a different panel of fields. Image upload modal. Inline editing in data tables.

### T2: PrestaShop Back Office

- **URL**: https://demo.prestashop.com/ (click "Back Office")
- **Credentials**: Auto-login
- **Stack**: PHP, Symfony, Twig, jQuery
- **Resets**: Periodically
- **UI complexity**: Product variant combination generator, pricing rule matrix, multi-step order management, translation interface, module marketplace, multi-store switcher
- **Key challenge**: Symfony forms with deeply nested fieldsets. Variant generator is a multi-step wizard with dynamic field generation. Heavy jQuery UI usage (sortable, datepicker, autocomplete).

### T3: TasteWP (WordPress Admin)

- **URL**: https://tastewp.com
- **Credentials**: Auto-generated on setup, no signup
- **Stack**: PHP, MySQL, React (Gutenberg)
- **Resets**: 48h (7 days if logged in)
- **UI complexity**: Gutenberg block editor (React-based, drag-and-drop, nested blocks, inline formatting toolbar), Customizer (live preview iframe + sidebar controls), plugin/theme installer, media library grid/list, user role editor, Settings API tabbed forms
- **Key challenge**: Gutenberg is a full React app embedded in WP admin. Blocks are contenteditable with floating toolbars. The Customizer is a split-pane with an iframe preview. Two very different UI paradigms in one app.

### T4: Odoo (Free Trial)

- **URL**: https://www.odoo.com/trial
- **Credentials**: Email signup, no CC, 15-day trial
- **Stack**: Python, PostgreSQL, OWL framework (custom JS)
- **Resets**: Trial expires after 15 days
- **UI complexity**: Kanban boards with drag-and-drop, pivot tables, calendar/Gantt views, many2many/one2many relational fields with inline sub-forms, chatter/activity feed, status bar workflows, report builder, dashboard widgets
- **Key challenge**: OWL framework renders everything client-side. Relational fields open inline forms within forms. Kanban drag-and-drop. Views switch between list/form/kanban/calendar dynamically. Custom widget types everywhere.

### T5: ERPNext (Frappe)

- **URL**: https://demo.erpnext.com (or current Frappe Cloud demo)
- **Credentials**: Auto-login sandbox
- **Stack**: Python, MariaDB, Frappe.js
- **Resets**: Regularly
- **UI complexity**: Doctype forms with child tables (editable grids inside forms), link fields with server-side search, workflow state transitions with button color changes, print format designer, report builder with dynamic filters, multi-level approval chains
- **Key challenge**: Frappe's desk UI is unique — sidebar navigation, breadcrumb trails, and a form/list view pattern unlike standard CRUD apps. Child tables are full editable grids embedded in forms. Link fields trigger server-side autocomplete.

### T6: Joomla Admin

- **URL**: https://launch.joomla.org/
- **Credentials**: Free signup, full admin access
- **Stack**: PHP, MySQL, Bootstrap
- **Resets**: Renew every 30 days
- **UI complexity**: Nested menu item management (each item type has different config fields), article editor with category/tag/media fields, module position assignment, template style overrides, ACL matrix, global configuration (4 tabs, dozens of options), batch processing dialog, custom fields with 15+ field types
- **Key challenge**: Menu item configuration changes dynamically based on selected menu item type. ACL is a permission matrix (groups x actions). Batch processing is a modal with multiple operations. Extension installer with drag-and-drop upload.

---

## Test Scenarios

Each target gets a standard set of scenarios. Not every scenario applies to every target.

### S1: Login
Navigate to login page, fill credentials, submit, verify successful login by checking for admin dashboard elements.

### S2: Map UI
Run `web.py map` on the admin dashboard. Score completeness of menu, toolbar, and context menu capture.

### S3: Navigate (breadcrumb/sidebar)
Navigate to 3+ different sections using sidebar/nav links. Verify each page loads and `ui` captures its elements. Use `back` to return.

### S4: Read data table
Find a data table (products, orders, users). Extract visible row data. Use pagination if available. Verify row count.

### S5: Create record (simple form)
Create a new record (product, article, customer) using a simple form — text fields, dropdowns, checkboxes. Save and verify.

### S6: Create record (complex form)
Create a record using a multi-tab or multi-step form with rich text, image upload, variant generation, or relational fields.

### S7: Edit record
Find an existing record, modify 2-3 fields, save, verify changes persisted.

### S8: Delete record
Delete a record. Handle confirmation dialog. Verify removal.

### S9: Search & filter
Use search/filter UI to find specific records. Verify filtered results.

### S10: Modal interaction
Trigger a modal/dialog (settings, confirmation, media picker). Interact with its contents. Close or confirm.

### S11: Context menu
Right-click on a relevant element. Capture and use context menu options.

### S12: Keyboard shortcuts
Use documented keyboard shortcuts (Ctrl+S to save, Ctrl+N for new, etc.). Verify actions triggered.

### S13: Error handling
Submit a form with missing required fields. Verify error messages appear and can be read via `ui`.

### S14: Multi-step workflow
Complete a multi-step process (e.g., create order > add items > set status > generate invoice).

---

## Scoring Rubric

Each scenario run is scored on 5 dimensions, 0-2 points each. **Max score per scenario: 10.**

| Dimension | 0 | 1 | 2 |
|-----------|---|---|---|
| **Discovery** | `ui`/`map` missed most elements | Found elements but some missing or mislabeled | All relevant elements found with correct labels |
| **Targeting** | Could not target elements (no usable selector) | Required coordinates or fragile selectors | Clean selector or text-based targeting worked |
| **Interaction** | Action failed (click missed, fill rejected, timeout) | Action worked with workaround (eval, retry, coordinate fallback) | Action worked on first try with standard command |
| **Verification** | Could not verify outcome | Needed screenshot to verify (not machine-readable) | `ui` or `eval` confirmed state change programmatically |
| **Resilience** | Broke on dynamic content, AJAX, or unexpected dialog | Needed manual waits or Escape to recover | Handled transitions, dialogs, and loading cleanly |

### Aggregate Scoring

Per target: sum all scenario scores, divide by max possible (scenarios attempted x 10).

| Rating | Score | Meaning |
|--------|-------|---------|
| **A** | 90-100% | Production-ready for this app type |
| **B** | 70-89% | Usable with minor workarounds |
| **C** | 50-69% | Functional but needs skill improvements |
| **D** | 30-49% | Major gaps, frequent fallbacks to eval/screenshots |
| **F** | 0-29% | Cannot complete basic journeys |

### Progress Tracking

Each test run records:

```
## Run <N> — <date>

Skill version: <version>
Target: <T1-T6>
Tester: <who>

| Scenario | Discovery | Targeting | Interaction | Verification | Resilience | Total | Notes |
|----------|-----------|-----------|-------------|--------------|------------|-------|-------|
| S1       |         2 |         2 |           2 |            2 |          2 |  10   |       |
| S2       |         1 |         - |           - |            - |          1 |   2   | missed sidebar |
| ...      |           |           |             |              |            |       |       |

**Score: XX/YY (ZZ%)**  Grade: B

### Gaps found
- ...

### Improvements made
- ...
```

---

## Run Log

_No runs recorded yet. First target: T1 (OpenCart)._
