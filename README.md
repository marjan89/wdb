# wdb — Web Debug Bridge

Browser automation CLI for debugging web admin panels. Single Go binary, zero dependencies.

Like `adb` for Android or `idb` for iOS — but for the web. Generic primitives that work across any admin panel: Joomla, Shopify, WordPress, Symfony, Laravel, PrestaShop.

## Install

```bash
go build -o wdb .
```

14MB binary. Bundles Chromium via [rod](https://github.com/go-rod/rod) — or connect to your own Chrome.

## Quick Start

```bash
# Launch browser
wdb launch https://your-site.com/admin

# Inspect the page
wdb ui                          # interactive elements
wdb map                         # navigation structure
wdb text h1                     # read any element's text

# Interact
wdb click '#login-btn'          # CSS selector
wdb click --text 'Save'         # by visible text
wdb fill '#email' 'user@example.com'
wdb press Enter

# Verify
wdb assert '.alert' 'saved'    # exits 0 on pass, 1 on fail

# Done
wdb stop
```

## Connect to Your Chrome

```bash
# Start Chrome with debug port
google-chrome --remote-debugging-port=9222 --user-data-dir=/tmp/wdb-profile

# Connect
wdb connect 9222
```

## Commands

### Inspection
```
ui [--all|--json|--filter x|--limit n]    Interactive element list
map                                        Navigation structure (sidebar, menus)
text <sel> [--all]                         Read visible text content
source [sel]                               Dump HTML (page or element)
url                                        Current URL + title
screenshot [path]                          Screenshot (default: /tmp/web_screen.png)
highlight <sel>                            Flash red overlay on element (2s)
```

### Interaction
```
click <sel>                                CSS selector
click --text 'Label'                       By visible text
click --text 'Save' --within '#sidebar'    Scoped to container
click --text 'Item' --nth 2                Pick 3rd match (0-indexed)
click --force <sel>                        JS click (ignores CSS visibility)
click --xy 400 300                         Coordinates (fallback)
fill <sel> <text>                          Clear + type (auto-detects TinyMCE/CKEditor)
type <text>                                Type into focused element
press <key>                                Keyboard (Enter, Tab, Escape, arrows, F1-F12)
select <sel> <option-text>                 Dropdown by visible text
hover <sel>                                Hover (or --text)
drag <from-sel> <to-sel>                   Drag and drop
upload <sel> <file...>                     File upload
```

### Navigation
```
navigate <url>                             Go to URL
navigate --link <sel>                      Extract href from element and navigate
back / forward / reload                    Browser navigation
scroll down|up|top|bottom [px]             Scroll (default 500px)
```

### Frames & Tabs
```
frames                                     List iframe tree with chain indices
frame 0                                    Enter first iframe
frame 0.0                                  Enter nested iframe (chain syntax)
frame main                                 Return to main frame
tabs                                       List all tabs
tabs 2                                     Switch to tab
tabs close                                 Close current tab
```

### State
```
cookie                                     List all cookies
cookie <name>                              Show cookie details
cookie --set name=value                    Set cookie
cookie --delete name                       Delete cookie
storage [key|--set k=v|--delete k]         localStorage (add --session for sessionStorage)
```

### Scripting
```
eval <js>                                  Run JavaScript (use arrow function syntax)
wait <sel|ms>                              Wait for element or milliseconds
assert <sel> <text>                        Verify element contains text (exit 0/1)
log [console|network|errors]               Stream browser events to stdout
```

### Global Flags
```
--timeout <ms>                             Element lookup timeout (default 5000)
```

## Log Streaming

Tail browser events while interacting — console messages, network requests, JS exceptions:

```bash
wdb log > /tmp/site.log &
wdb click '#save'
wdb assert '.alert' 'saved'
kill %1
cat /tmp/site.log
```

Output:
```
14:23:45.012 [net] 200 https://example.com/api/save (application/json)
14:23:45.150 [console] ERROR: Validation failed for field "email"
14:23:46.003 [error] TypeError: Cannot read properties of null
```

Filter by type: `log console`, `log network`, `log errors`.

## Rich Text Editors

`fill` auto-detects TinyMCE and CKEditor. If the target textarea is hidden behind a rich text editor, wdb uses the editor's API automatically:

```bash
wdb fill '#article-body' 'Article content here'
# Output: Filled #article-body (via tinymce)
```

## How It Works

wdb connects to Chrome/Chromium via the Chrome DevTools Protocol (CDP). Each command is a standalone process that reconnects (~50ms overhead). Browser state persists between commands via a CDP URL file at `/tmp/web-control-cdp.txt`.

Built with [rod](https://github.com/go-rod/rod) for Go.

## License

MIT
