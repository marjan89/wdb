package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	cdpFile   = "/tmp/web-control-cdp.txt"
	frameFile = "/tmp/web-control-frame.txt"
	pageFile  = "/tmp/web-control-page.txt"
)

var globalTimeout = 5 * time.Second

// ── Element extraction JS (same as web.py) ────────────────────
const extractJS = `() => {
	const seen = new Set();
	const results = [];
	function walk(root) {
		const els = root.querySelectorAll([
			'a[href]','a.btn','a[data-toggle]','a[data-action]','a[data-bs-toggle]',
			'button','input','select','textarea',
			'[role="button"]','[role="link"]','[role="tab"]',
			'[role="menuitem"]','[role="checkbox"]','[role="radio"]',
			'[role="switch"]','[role="combobox"]','[role="textbox"]',
			'[tabindex]:not([tabindex="-1"])',
			'[contenteditable="true"]','summary',
		].join(','));
		for (const el of els) {
			if (seen.has(el)) continue;
			seen.add(el);
			const rect = el.getBoundingClientRect();
			if (rect.width < 4 || rect.height < 4) continue;
			const style = window.getComputedStyle(el);
			if (style.display==='none'||style.visibility==='hidden') continue;
			if (parseFloat(style.opacity)===0) continue;
			const tag = el.tagName.toLowerCase();
			const role = el.getAttribute('role')||'';
			const type = el.getAttribute('type')||'';
			const id = el.id||'';
			const name = el.getAttribute('name')||'';
			const ariaLabel = el.getAttribute('aria-label')||'';
			const placeholder = el.getAttribute('placeholder')||'';
			const href = tag==='a'?(el.getAttribute('href')||''):'';
			const disabled = el.disabled||el.getAttribute('aria-disabled')==='true';
			const checked = el.checked||el.getAttribute('aria-checked')==='true';
			const value = (tag==='select'&&el.selectedOptions&&el.selectedOptions.length)
				? el.selectedOptions[0].text
				: (el.value||'').substring(0,80);
			let text = ariaLabel;
			if(!text){
				if(tag==='input'||tag==='select'||tag==='textarea'){text=placeholder}
				else{text=(el.textContent||'').trim().replace(/\s+/g,' ').substring(0,80)}
			}
			results.push({tag,role,type,id,name,text,placeholder,value,
				href:href.substring(0,120),checked,disabled,
				x:Math.round(rect.x+rect.width/2),
				y:Math.round(rect.y+rect.height/2),
				w:Math.round(rect.width),h:Math.round(rect.height),
				inViewport:rect.top<window.innerHeight&&rect.bottom>0
					&&rect.left<window.innerWidth&&rect.right>0,
			});
		}
		for(const el of root.querySelectorAll('*')){
			if(el.shadowRoot) walk(el.shadowRoot);
		}
	}
	walk(document);
	results.sort((a,b)=>a.y!==b.y?a.y-b.y:a.x-b.x);
	const deduped=[];
	for(const r of results){
		const dup=deduped.find(d=>Math.abs(d.x-r.x)<5&&Math.abs(d.y-r.y)<5);
		if(!dup) deduped.push(r);
		else if(r.text&&!dup.text) Object.assign(dup,r);
	}
	return deduped;
}`

type element struct {
	Tag        string `json:"tag"`
	Role       string `json:"role"`
	Type       string `json:"type"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Text       string `json:"text"`
	Placeholder string `json:"placeholder"`
	Value      string `json:"value"`
	Href       string `json:"href"`
	Checked    bool   `json:"checked"`
	Disabled   bool   `json:"disabled"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	W          int    `json:"w"`
	H          int    `json:"h"`
	InViewport bool   `json:"inViewport"`
}

func elementKind(e element) string {
	if e.Tag == "a" { return "link" }
	if e.Tag == "button" || e.Role == "button" { return "btn" }
	if e.Tag == "input" {
		if e.Type != "" { return e.Type }
		return "text"
	}
	if e.Tag == "select" { return "select" }
	if e.Tag == "textarea" { return "textarea" }
	if e.Role != "" { return e.Role }
	return e.Tag
}

func formatElement(e element) string {
	kind := elementKind(e)
	marker := "*"
	if e.Disabled { marker = "~" }

	var parts []string
	if e.Text != "" {
		t := e.Text
		if len(t) > 60 { t = t[:60] }
		parts = append(parts, t)
	} else if e.Placeholder != "" {
		p := e.Placeholder
		if len(p) > 40 { p = p[:40] }
		parts = append(parts, `"`+p+`"`)
	}
	if (e.Tag == "input" || e.Tag == "textarea" || e.Tag == "select") && e.Value != "" {
		v := e.Value
		if len(v) > 30 { v = v[:30] }
		parts = append(parts, `= "`+v+`"`)
	}
	if e.Checked { parts = append(parts, "[x]") }
	if e.Tag == "a" && e.Href != "" && !strings.HasPrefix(e.Href, "javascript:") && e.Href != "#" {
		h := e.Href
		if len(h) > 50 { h = h[:50] }
		parts = append(parts, "-> "+h)
	}

	label := "(no label)"
	if len(parts) > 0 { label = strings.Join(parts, "  ") }

	sel := ""
	if e.ID != "" { sel = "  #" + e.ID }
	if sel == "" && e.Name != "" { sel = "  [" + e.Name + "]" }

	return fmt.Sprintf("%s (%4d,%4d)  [%-8s]  %s%s", marker, e.X, e.Y, kind, label, sel)
}

// ── Browser connection ────────────────────────────────────────

func connectBrowser() *rod.Browser {
	data, err := os.ReadFile(cdpFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: no browser running. Use 'wdb launch' first.")
		os.Exit(1)
	}
	wsURL := strings.TrimSpace(string(data))
	browser := rod.New().ControlURL(wsURL).MustConnect()
	return browser
}

func getPage(browser *rod.Browser) *rod.Page {
	pages := browser.MustPages()
	// Check if a specific tab was selected via `tabs N`
	if data, err := os.ReadFile(pageFile); err == nil {
		idx, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && idx >= 0 && idx < len(pages) {
			return pages[idx]
		}
	}
	// Fallback: last non-blank page
	for i := len(pages) - 1; i >= 0; i-- {
		info := pages[i].MustInfo()
		if info.URL != "about:blank" && !strings.HasPrefix(info.URL, "chrome://") {
			return pages[i]
		}
	}
	if len(pages) > 0 { return pages[0] }
	return browser.MustPage("")
}

func getFrame(page *rod.Page) *rod.Page {
	data, err := os.ReadFile(frameFile)
	if err != nil { return page }
	spec := strings.TrimSpace(string(data))
	if spec == "" || spec == "main" { return page }

	// Try chain syntax (e.g., "0.0", "1.2.0") or single index
	if parts := strings.Split(spec, "."); len(parts) >= 1 {
		allDigits := true
		for _, p := range parts {
			if _, err := strconv.Atoi(p); err != nil {
				allDigits = false
				break
			}
		}
		if allDigits {
			current := page
			for _, p := range parts {
				idx, _ := strconv.Atoi(p)
				iframes, _ := current.Elements("iframe")
				if idx < 0 || idx >= len(iframes) {
					fmt.Fprintf(os.Stderr, "ERROR: frame index %d out of range (%d iframes)\n", idx, len(iframes))
					os.Exit(1)
				}
				frame, err := iframes[idx].Frame()
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: cannot access frame %d: %v\n", idx, err)
					os.Exit(1)
				}
				current = frame
			}
			return current
		}
	}

	// Try by URL substring — walk iframes recursively
	var findFrame func(p *rod.Page, url string) *rod.Page
	findFrame = func(p *rod.Page, url string) *rod.Page {
		iframes, _ := p.Elements("iframe")
		for _, el := range iframes {
			frame, err := el.Frame()
			if err != nil { continue }
			info := frame.MustInfo()
			if strings.Contains(info.URL, url) {
				return frame
			}
			if found := findFrame(frame, url); found != nil {
				return found
			}
		}
		return nil
	}
	if found := findFrame(page, spec); found != nil {
		return found
	}
	return page
}

func findElement(frame *rod.Page, selector string) *rod.Element {
	el, err := frame.Timeout(globalTimeout).Element(selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: element not found: %s\n", selector)
		os.Exit(1)
	}
	return el
}

func findElementByText(frame *rod.Page, text string) *rod.Element {
	el, err := frame.Timeout(globalTimeout).ElementR("*", text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: no element matching text: %s\n", text)
		os.Exit(1)
	}
	return el
}

// ── Commands ──────────────────────────────────────────────────

func cmdLaunch(args []string) {
	url := ""
	if len(args) > 0 { url = args[0] }

	// Find Chrome — Leakless(false) so browser survives after this process exits
	u := launcher.New().
		Leakless(false).
		Headless(false).
		Set("no-first-run").
		Set("no-default-browser-check").
		MustLaunch()

	browser := rod.New().ControlURL(u).MustConnect()

	// Save CDP URL
	os.WriteFile(cdpFile, []byte(u), 0644)
	fmt.Printf("Browser launched, CDP: %s\n", u[:60]+"...")

	if url != "" {
		page := browser.MustPage(url)
		page.MustWaitDOMStable()
		fmt.Printf("  -> %s\n", url)
	}
}

func cmdStop(args []string) {
	browser := connectBrowser()
	browser.MustClose()
	os.Remove(cdpFile)
	os.Remove(frameFile)
	os.Remove(pageFile)
	fmt.Println("Browser closed.")
}

func cmdUI(args []string) {
	showAll := false
	showJSON := false
	var filter string
	limit := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all": showAll = true
		case "--json": showJSON = true
		case "--filter":
			if i+1 < len(args) { i++; filter = strings.ToLower(args[i]) }
		case "--limit":
			if i+1 < len(args) { i++; limit, _ = strconv.Atoi(args[i]) }
		}
	}

	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	var elements []element
	result := frame.MustEval(extractJS)
	raw, err := result.MarshalJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR marshaling result: %v\n", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(raw, &elements); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR parsing elements: %v\n", err)
		os.Exit(1)
	}

	if showJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(elements)
		return
	}

	info := page.MustInfo()
	frameInfo := frame.MustInfo()
	fmt.Printf("-- %s\n   %s\n\n", info.Title, frameInfo.URL)

	count := 0
	offscreen := 0
	for _, e := range elements {
		if !e.InViewport && !showAll { offscreen++; continue }
		line := formatElement(e)
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) { continue }
		if limit > 0 && count >= limit { break }
		fmt.Println(line)
		count++
	}
	if offscreen > 0 {
		fmt.Printf("\n  (%d off-screen)\n", offscreen)
	}
}

func cmdFrames(args []string) {
	browser := connectBrowser()
	page := getPage(browser)

	activeSpec := ""
	if data, err := os.ReadFile(frameFile); err == nil {
		activeSpec = strings.TrimSpace(string(data))
	}

	mainURL := page.MustInfo().URL
	if len(mainURL) > 90 { mainURL = mainURL[:90] }
	fmt.Printf("  [main] %s\n", mainURL)

	var listFrames func(p *rod.Page, depth int, prefix string)
	listFrames = func(p *rod.Page, depth int, prefix string) {
		iframes, _ := p.Elements("iframe")
		for i, el := range iframes {
			chain := strconv.Itoa(i)
			if prefix != "" {
				chain = prefix + "." + strconv.Itoa(i)
			}
			frame, err := el.Frame()
			if err != nil {
				fmt.Printf("%s  [%s] (inaccessible)\n", strings.Repeat("  ", depth), chain)
				continue
			}
			info := frame.MustInfo()
			marker := " "
			if activeSpec == chain {
				marker = ">"
			} else if activeSpec != "" && strings.Contains(info.URL, activeSpec) {
				marker = ">"
			}
			u := info.URL
			if len(u) > 90 { u = u[:90] }
			fmt.Printf("%s%s [%s] %s\n", strings.Repeat("  ", depth), marker, chain, u)
			listFrames(frame, depth+1, chain)
		}
	}
	listFrames(page, 0, "")
}

func cmdFrame(args []string) {
	if len(args) == 0 || args[0] == "main" {
		os.Remove(frameFile)
		fmt.Println("Reset to main frame")
		return
	}
	os.WriteFile(frameFile, []byte(args[0]), 0644)
	fmt.Printf("Switched to frame: %s\n", args[0])
}

func cmdNavigate(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdb navigate <url|--link sel>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	if args[0] == "--link" && len(args) > 1 {
		el := findElement(frame, args[1])
		href := el.MustEval(`function() { return this.href || this.getAttribute('href') || ''; }`).Str()
		if href == "" {
			fmt.Fprintln(os.Stderr, "ERROR: element has no href")
			os.Exit(1)
		}
		frame.MustNavigate(href).MustWaitDOMStable()
		info := frame.MustInfo()
		fmt.Printf("-> %s\n   %s\n", info.URL, info.Title)
		return
	}

	page.MustNavigate(args[0]).MustWaitDOMStable()
	info := page.MustInfo()
	fmt.Printf("-> %s\n   %s\n", info.URL, info.Title)
}

func cmdClick(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdb click <selector>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	// Parse flags
	force := false
	nth := 0
	var within string
	var filtered []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--force" {
			force = true
		} else if args[i] == "--within" && i+1 < len(args) {
			i++
			within = args[i]
		} else if args[i] == "--nth" && i+1 < len(args) {
			i++
			nth, _ = strconv.Atoi(args[i])
		} else {
			filtered = append(filtered, args[i])
		}
	}
	args = filtered

	if args[0] == "--text" && len(args) > 1 {
		var el *rod.Element
		if within != "" {
			// Search clickable elements within container
			parent := findElement(frame, within)
			var err error
			el, err = parent.ElementR("a, button, [role=button], [role=link], [role=menuitem], [role=tab]", args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: no element matching text %q within %s\n", args[1], within)
				os.Exit(1)
			}
		} else if nth > 0 {
			// JS-based nth match — click directly and return
			frame.MustEval(fmt.Sprintf(`(nth) => {
				const re = new RegExp(%s);
				const els = document.querySelectorAll('a, button, [role="button"], [role="link"], [role="menuitem"], span, label, li');
				const matches = [];
				for (const el of els) {
					if (re.test(el.textContent.trim()) && el.offsetParent !== null) matches.push(el);
				}
				if (nth >= matches.length) throw new Error('--nth ' + nth + ' but only ' + matches.length + ' matches');
				matches[nth].click();
			}`, strconv.Quote(args[1])), nth)
			fmt.Printf("Clicked text: %s (nth: %d)\n", args[1], nth)
			return
		} else {
			el = findElementByText(frame, args[1])
		}
		if force {
			el.MustEval(`function() { this.click(); }`)
		} else {
			el.MustClick()
		}
		fmt.Printf("Clicked text: %s\n", args[1])
	} else if args[0] == "--xy" && len(args) > 2 {
		x, _ := strconv.ParseFloat(args[1], 64)
		y, _ := strconv.ParseFloat(args[2], 64)
		page.Mouse.MustMoveTo(x, y)
		page.Mouse.MustDown("left")
		page.Mouse.MustUp("left")
		fmt.Printf("Clicked (%s, %s)\n", args[1], args[2])
	} else {
		el := findElement(frame, args[0])
		if force {
			el.MustEval(`function() { this.click(); }`)
		} else {
			el.MustClick()
		}
		fmt.Printf("Clicked: %s\n", args[0])
	}
}

func cmdFill(args []string) {
	if len(args) < 2 { fmt.Fprintln(os.Stderr, "Usage: wdb fill <selector> <text>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	sel := args[0]
	text := strings.Join(args[1:], " ")

	el := findElement(frame, sel)

	// Check if the element is hidden (rich text editor replaced it)
	visible := el.MustEval(`function() { return this.offsetParent !== null && getComputedStyle(this).display !== 'none'; }`).Bool()
	if !visible {
		// Try TinyMCE, CKEditor 4, CKEditor 5
		result := frame.MustEval(fmt.Sprintf(`(content) => {
			const id = %s;
			// TinyMCE
			if (typeof tinyMCE !== 'undefined') {
				const ed = tinyMCE.get(id);
				if (ed) { ed.setContent('<p>' + content + '</p>'); return 'tinymce'; }
			}
			// CKEditor 4
			if (typeof CKEDITOR !== 'undefined' && CKEDITOR.instances[id]) {
				CKEDITOR.instances[id].setData('<p>' + content + '</p>');
				return 'ckeditor4';
			}
			// CKEditor 5 — stored on the element
			const ta = document.getElementById(id);
			if (ta && ta.ckeditorInstance) {
				ta.ckeditorInstance.setData('<p>' + content + '</p>');
				return 'ckeditor5';
			}
			// Fallback: set textarea value directly
			if (ta) { ta.value = content; return 'textarea'; }
			return 'not found';
		}`, strconv.Quote(el.MustProperty("id").Str())), text)
		fmt.Printf("Filled %s (via %s)\n", sel, result.Str())
		return
	}

	el.MustSelectAllText().MustInput(text)
	fmt.Printf("Filled %s\n", sel)
}

func cmdType(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdbtype <text>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	page.MustInsertText(strings.Join(args, " "))
	fmt.Println("Typed")
}

var keyNames = map[string]input.Key{
	"enter": input.Enter, "tab": input.Tab, "escape": input.Escape, "esc": input.Escape,
	"backspace": input.Backspace, "delete": input.Delete, "space": input.Space,
	"arrowup": input.ArrowUp, "arrowdown": input.ArrowDown,
	"arrowleft": input.ArrowLeft, "arrowright": input.ArrowRight,
	"up": input.ArrowUp, "down": input.ArrowDown, "left": input.ArrowLeft, "right": input.ArrowRight,
	"home": input.Home, "end": input.End, "pageup": input.PageUp, "pagedown": input.PageDown,
	"insert": input.Insert,
	"f1": input.F1, "f2": input.F2, "f3": input.F3, "f4": input.F4,
	"f5": input.F5, "f6": input.F6, "f7": input.F7, "f8": input.F8,
	"f9": input.F9, "f10": input.F10, "f11": input.F11, "f12": input.F12,
	"shift": input.ShiftLeft, "control": input.ControlLeft, "ctrl": input.ControlLeft,
	"alt": input.AltLeft, "meta": input.MetaLeft, "cmd": input.MetaLeft,
}

func cmdPress(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdb press <key>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)

	name := strings.ToLower(args[0])
	if k, ok := keyNames[name]; ok {
		page.Keyboard.MustType(k)
		fmt.Printf("Pressed: %s\n", args[0])
		return
	}

	// Single character — type it as a key
	if len(args[0]) == 1 {
		r := rune(args[0][0])
		page.Keyboard.MustType(input.Key(r))
		fmt.Printf("Pressed: %s\n", args[0])
		return
	}

	fmt.Fprintf(os.Stderr, "ERROR: unknown key %q\n", args[0])
	os.Exit(1)
}

func cmdEval(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdbeval <js>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	result := frame.MustEval(strings.Join(args, " "))
	fmt.Println(result.String())
}

func cmdURL(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	info := page.MustInfo()
	fmt.Printf("URL:   %s\nTitle: %s\n", info.URL, info.Title)
}

func cmdScroll(args []string) {
	dir := "down"
	amount := 500
	if len(args) > 0 { dir = args[0] }
	if len(args) > 1 { amount, _ = strconv.Atoi(args[1]) }
	browser := connectBrowser()
	page := getPage(browser)
	switch dir {
	case "down":
		page.Mouse.MustScroll(0, float64(amount))
	case "up":
		page.Mouse.MustScroll(0, -float64(amount))
	case "top":
		page.MustEval(`() => window.scrollTo(0,0)`)
	case "bottom":
		page.MustEval(`() => window.scrollTo(0,document.body.scrollHeight)`)
	}
	fmt.Printf("Scrolled %s\n", dir)
}

func cmdWait(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdbwait <selector|ms>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	if ms, err := strconv.Atoi(args[0]); err == nil {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		fmt.Printf("Waited %dms\n", ms)
	} else {
		findElement(frame, args[0])
		fmt.Printf("Found: %s\n", args[0])
	}
}

func cmdScreenshot(args []string) {
	path := "/tmp/web_screen.png"
	if len(args) > 0 { path = args[0] }
	browser := connectBrowser()
	page := getPage(browser)
	page.MustScreenshotFullPage(path)
	fmt.Printf("Screenshot: %s\n", path)
}

func cmdSelect(args []string) {
	if len(args) < 2 { fmt.Fprintln(os.Stderr, "Usage: wdb select <selector> <option-text...>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	el := findElement(frame, args[0])
	el.MustSelect(args[1:]...)
	fmt.Printf("Selected: %s\n", strings.Join(args[1:], ", "))
}

func cmdHover(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdb hover <selector>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	if args[0] == "--text" && len(args) > 1 {
		findElementByText(frame, args[1]).MustHover()
		fmt.Printf("Hovered text: %s\n", args[1])
	} else {
		findElement(frame, args[0]).MustHover()
		fmt.Printf("Hovered: %s\n", args[0])
	}
}

func cmdBack(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	page.MustNavigateBack()
	page.MustWaitDOMStable()
	info := page.MustInfo()
	fmt.Printf("<- %s\n", info.URL)
}

func cmdForward(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	page.MustNavigateForward()
	page.MustWaitDOMStable()
	info := page.MustInfo()
	fmt.Printf("-> %s\n", info.URL)
}

func cmdReload(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	page.MustReload()
	page.MustWaitDOMStable()
	fmt.Println("Reloaded")
}

func cmdTabs(args []string) {
	browser := connectBrowser()
	pages := browser.MustPages()

	if len(args) > 0 && args[0] == "close" {
		page := getPage(browser)
		page.MustClose()
		os.Remove(pageFile)
		fmt.Println("Closed current tab")
		return
	}

	if len(args) > 0 {
		// Switch to tab by index
		idx, err := strconv.Atoi(args[0])
		if err != nil || idx < 0 || idx >= len(pages) {
			fmt.Fprintf(os.Stderr, "ERROR: invalid tab index %q (have %d tabs)\n", args[0], len(pages))
			os.Exit(1)
		}
		pages[idx].MustActivate()
		os.WriteFile(pageFile, []byte(strconv.Itoa(idx)), 0644)
		info := pages[idx].MustInfo()
		fmt.Printf("Switched to tab %d: %s\n", idx, info.Title)
		return
	}

	// List tabs
	activePage := getPage(browser)
	activeURL := activePage.MustInfo().URL
	for i, p := range pages {
		info := p.MustInfo()
		marker := " "
		if info.URL == activeURL { marker = ">" }
		u := info.URL
		if len(u) > 80 { u = u[:80] }
		title := info.Title
		if len(title) > 40 { title = title[:40] }
		fmt.Printf("%s [%d] %s  %s\n", marker, i, title, u)
	}
}

func cmdUpload(args []string) {
	if len(args) < 2 { fmt.Fprintln(os.Stderr, "Usage: wdb upload <selector> <file...>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	el := findElement(frame, args[0])
	el.MustSetFiles(args[1:]...)
	fmt.Printf("Uploaded %d file(s) to %s\n", len(args)-1, args[0])
}

func cmdConnect(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: wdb connect <port>")
		os.Exit(1)
	}
	wsURL := args[0]
	if !strings.HasPrefix(wsURL, "ws") {
		resolved, err := launcher.ResolveURL("ws://127.0.0.1:" + args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: cannot reach browser on port %s: %v\n", args[0], err)
			os.Exit(1)
		}
		wsURL = resolved
	}
	browser := rod.New().ControlURL(wsURL).MustConnect()
	pages := browser.MustPages()
	os.WriteFile(cdpFile, []byte(wsURL), 0644)
	fmt.Printf("Connected (%d tabs)\n", len(pages))
}

func cmdDrag(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wdb drag <from-sel> <to-sel>")
		os.Exit(1)
	}
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	src := findElement(frame, args[0])
	dst := findElement(frame, args[1])

	src.MustScrollIntoView()
	srcShape := src.MustShape()
	sq := srcShape.Quads[0]
	sx := (sq[0] + sq[2] + sq[4] + sq[6]) / 4
	sy := (sq[1] + sq[3] + sq[5] + sq[7]) / 4

	dstShape := dst.MustShape()
	dq := dstShape.Quads[0]
	dx := (dq[0] + dq[2] + dq[4] + dq[6]) / 4
	dy := (dq[1] + dq[3] + dq[5] + dq[7]) / 4

	page.Mouse.MustMoveTo(sx, sy)
	time.Sleep(100 * time.Millisecond)
	page.Mouse.MustDown("left")
	time.Sleep(100 * time.Millisecond)

	steps := 20
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		page.Mouse.MustMoveTo(sx+(dx-sx)*t, sy+(dy-sy)*t)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	page.Mouse.MustUp("left")
	fmt.Printf("Dragged %s -> %s\n", args[0], args[1])
}

func cmdAssert(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wdb assert <selector> <expected-text>")
		os.Exit(1)
	}
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	el := findElement(frame, args[0])
	text, err := el.Text()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read text: %v\n", err)
		os.Exit(1)
	}

	expected := strings.Join(args[1:], " ")
	if strings.Contains(text, expected) {
		fmt.Printf("PASS: %s contains %q\n", args[0], expected)
	} else {
		actual := text
		if len(actual) > 200 {
			actual = actual[:200] + "..."
		}
		fmt.Fprintf(os.Stderr, "FAIL: %s does not contain %q\n", args[0], expected)
		fmt.Fprintf(os.Stderr, "  got: %s\n", actual)
		os.Exit(1)
	}
}

func cmdCookie(args []string) {
	browser := connectBrowser()
	page := getPage(browser)

	// Delete
	if len(args) >= 2 && args[0] == "--delete" {
		name := args[1]
		cookies, err := page.Cookies(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		for _, c := range cookies {
			if c.Name == name {
				err := proto.NetworkDeleteCookies{Name: c.Name, Domain: c.Domain, Path: c.Path}.Call(page)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Deleted: %s\n", name)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "ERROR: cookie not found: %s\n", name)
		os.Exit(1)
	}

	// Set
	if len(args) >= 2 && args[0] == "--set" {
		parts := strings.SplitN(args[1], "=", 2)
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: wdb cookie --set name=value")
			os.Exit(1)
		}
		info := page.MustInfo()
		page.MustSetCookies(&proto.NetworkCookieParam{
			Name:  parts[0],
			Value: parts[1],
			URL:   info.URL,
		})
		fmt.Printf("Set: %s=%s\n", parts[0], parts[1])
		return
	}

	// Get specific
	cookies, err := page.Cookies(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if len(args) == 1 {
		for _, c := range cookies {
			if c.Name == args[0] {
				fmt.Printf("Name:     %s\nValue:    %s\nDomain:   %s\nPath:     %s\nSecure:   %v\nHttpOnly: %v\n",
					c.Name, c.Value, c.Domain, c.Path, c.Secure, c.HTTPOnly)
				if c.Expires > 0 {
					fmt.Printf("Expires:  %s\n", time.Unix(int64(c.Expires), 0).Format(time.RFC3339))
				}
				return
			}
		}
		fmt.Fprintf(os.Stderr, "ERROR: cookie not found: %s\n", args[0])
		os.Exit(1)
	}

	// List all
	if len(cookies) == 0 {
		fmt.Println("No cookies.")
		return
	}
	for _, c := range cookies {
		v := c.Value
		if len(v) > 60 {
			v = v[:60] + "..."
		}
		fmt.Printf("%-30s  %s\n", c.Name, v)
	}
	fmt.Printf("\n(%d cookies)\n", len(cookies))
}

// ── Map JS ───────────────────────────────────────────────────

const mapJS = `() => {
	const sections = [];
	const seen = new Set();
	function walkUl(ul) {
		const items = [];
		for (const li of ul.children) {
			if (li.tagName !== 'LI') continue;
			const a = li.querySelector(':scope > a, :scope > button, :scope > span > a');
			if (!a || seen.has(a)) continue;
			seen.add(a);
			const text = (a.getAttribute('aria-label') || a.textContent || '').trim().replace(/\s+/g, ' ').substring(0, 80);
			if (!text) continue;
			const href = a.tagName === 'A' ? (a.getAttribute('href') || '').substring(0, 120) : '';
			const sub = li.querySelector(':scope > ul, :scope > ol');
			const children = sub ? walkUl(sub) : [];
			items.push({text, href, children});
		}
		return items;
	}
	const navs = document.querySelectorAll('nav, [role="navigation"], [role="menubar"], aside, [role="complementary"]');
	for (const nav of navs) {
		if (seen.has(nav)) continue;
		seen.add(nav);
		const label = nav.getAttribute('aria-label') || nav.id || nav.tagName.toLowerCase();
		const ul = nav.querySelector('ul, ol');
		if (!ul) continue;
		const items = walkUl(ul);
		if (items.length) sections.push({label, items});
	}
	return sections;
}`

type mapItem struct {
	Text     string    `json:"text"`
	Href     string    `json:"href"`
	Children []mapItem `json:"children"`
}

type mapSection struct {
	Label string    `json:"label"`
	Items []mapItem `json:"items"`
}

func printMapItems(items []mapItem, indent int) {
	for _, item := range items {
		prefix := strings.Repeat("  ", indent)
		fmt.Printf("%s%s\n", prefix, item.Text)
		printMapItems(item.Children, indent+1)
	}
}

func cmdMap(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	result := frame.MustEval(mapJS)
	raw, err := result.MarshalJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	var sections []mapSection
	if err := json.Unmarshal(raw, &sections); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if len(sections) == 0 {
		fmt.Println("No navigation structures found.")
		return
	}

	for _, s := range sections {
		fmt.Printf("── %s ──\n", s.Label)
		printMapItems(s.Items, 1)
		fmt.Println()
	}
}

func cmdLog(args []string) {
	browser := connectBrowser()
	page := getPage(browser)

	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}

	proto.RuntimeEnable{}.Call(page)
	if filter == "" || filter == "network" {
		proto.NetworkEnable{}.Call(page)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "\nStopped.")
		cancel()
	}()

	pCtx := page.Context(ctx)
	fmt.Fprintln(os.Stderr, "Streaming logs (Ctrl+C to stop)...")

	var callbacks []interface{}

	if filter == "" || filter == "console" || filter == "errors" {
		callbacks = append(callbacks, func(e *proto.RuntimeConsoleAPICalled) {
			if filter == "errors" && string(e.Type) != "error" && string(e.Type) != "warning" {
				return
			}
			var parts []string
			for _, arg := range e.Args {
				if !arg.Value.Nil() {
					parts = append(parts, arg.Value.String())
				} else if arg.Description != "" {
					parts = append(parts, arg.Description)
				}
			}
			level := strings.ToUpper(string(e.Type))
			fmt.Printf("[console] %s: %s\n", level, strings.Join(parts, " "))
		})
	}

	if filter == "" || filter == "errors" {
		callbacks = append(callbacks, func(e *proto.RuntimeExceptionThrown) {
			text := e.ExceptionDetails.Text
			if e.ExceptionDetails.Exception != nil && e.ExceptionDetails.Exception.Description != "" {
				text = e.ExceptionDetails.Exception.Description
			}
			fmt.Printf("[error] %s\n", text)
		})
	}

	if filter == "" || filter == "network" {
		callbacks = append(callbacks, func(e *proto.NetworkResponseReceived) {
			r := e.Response
			u := r.URL
			if len(u) > 80 {
				u = u[:80] + "..."
			}
			fmt.Printf("[net] %d %s (%s)\n", r.Status, u, r.MIMEType)
		})
	}

	if len(callbacks) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: unknown filter %q (use: console, network, errors)\n", filter)
		os.Exit(1)
	}

	wait := pCtx.EachEvent(callbacks...)
	wait()
}

func cmdStorage(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	storageType := "localStorage"
	var filtered []string
	for _, a := range args {
		if a == "--session" {
			storageType = "sessionStorage"
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	// Delete
	if len(args) >= 2 && args[0] == "--delete" {
		frame.MustEval(fmt.Sprintf(`() => %s.removeItem(%q)`, storageType, args[1]))
		fmt.Printf("Deleted: %s\n", args[1])
		return
	}

	// Set
	if len(args) >= 2 && args[0] == "--set" {
		parts := strings.SplitN(args[1], "=", 2)
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: wdb storage --set key=value")
			os.Exit(1)
		}
		frame.MustEval(fmt.Sprintf(`() => %s.setItem(%q, %q)`, storageType, parts[0], parts[1]))
		fmt.Printf("Set: %s=%s\n", parts[0], parts[1])
		return
	}

	// Get specific
	if len(args) == 1 {
		result := frame.MustEval(fmt.Sprintf(`() => %s.getItem(%q)`, storageType, args[0]))
		if result.Nil() {
			fmt.Fprintf(os.Stderr, "ERROR: key not found: %s\n", args[0])
			os.Exit(1)
		}
		fmt.Println(result.Str())
		return
	}

	// List all
	result := frame.MustEval(fmt.Sprintf(`() => {
		const items = [];
		for (let i = 0; i < %s.length; i++) {
			const key = %s.key(i);
			const val = %s.getItem(key);
			items.push({key, val});
		}
		return items;
	}`, storageType, storageType, storageType))

	raw, _ := result.MarshalJSON()
	var items []struct {
		Key string `json:"key"`
		Val string `json:"val"`
	}
	json.Unmarshal(raw, &items)

	if len(items) == 0 {
		fmt.Printf("No %s items.\n", storageType)
		return
	}
	for _, item := range items {
		v := item.Val
		if len(v) > 60 {
			v = v[:60] + "..."
		}
		fmt.Printf("%-30s  %s\n", item.Key, v)
	}
	fmt.Printf("\n(%d items)\n", len(items))
}

func cmdHighlight(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: wdb highlight <selector>")
		os.Exit(1)
	}
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	el := findElement(frame, args[0])
	el.MustEval(`function() {
		const box = this.getBoundingClientRect();
		const overlay = document.createElement('div');
		overlay.style.cssText = 'position:fixed;z-index:999999;pointer-events:none;' +
			'border:3px solid red;background:rgba(255,0,0,0.15);' +
			'top:'+box.top+'px;left:'+box.left+'px;width:'+box.width+'px;height:'+box.height+'px;' +
			'transition:opacity 0.5s;';
		document.body.appendChild(overlay);
		setTimeout(function(){ overlay.style.opacity = '0'; }, 1500);
		setTimeout(function(){ overlay.remove(); }, 2000);
	}`)
	fmt.Printf("Highlighted: %s\n", args[0])
}

func cmdText(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: wdb text <selector>")
		os.Exit(1)
	}

	all := false
	var filtered []string
	for _, a := range args {
		if a == "--all" {
			all = true
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: wdb text <selector>")
		os.Exit(1)
	}

	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	if all {
		els, err := frame.Timeout(globalTimeout).Elements(args[0])
		if err != nil || len(els) == 0 {
			fmt.Fprintf(os.Stderr, "ERROR: no elements found: %s\n", args[0])
			os.Exit(1)
		}
		for i, el := range els {
			text, err := el.Text()
			if err != nil {
				continue
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			fmt.Printf("[%d] %s\n", i, text)
		}
		return
	}

	el := findElement(frame, args[0])
	text, err := el.Text()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read text: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(text)
}

func cmdSource(args []string) {
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	if len(args) == 0 {
		html, err := frame.HTML()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: cannot read HTML: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(html)
		return
	}
	el := findElement(frame, args[0])
	html, err := el.HTML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read HTML: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(html)
}

// ── Main ──────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		fmt.Println("wdb - Web Debug Bridge (browser control CLI)")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  launch [url]        Launch browser")
		fmt.Println("  connect <port>      Attach to debug port")
		fmt.Println("  stop                Close browser")
		fmt.Println("  ui [--all|--json|--filter x|--limit n]")
		fmt.Println("  map                 Show navigation structure")
		fmt.Println("  click <sel>         Click (--text/--xy/--force/--within/--nth)")
		fmt.Println("  fill <sel> <text>   Clear + type")
		fmt.Println("  type <text>         Type into focused")
		fmt.Println("  press <key>         Press key (Enter, Tab, Escape...)")
		fmt.Println("  select <sel> <opt>  Select dropdown option by text")
		fmt.Println("  hover <sel>         Hover (or --text)")
		fmt.Println("  navigate <url|--link sel>  Go to URL or follow link")
		fmt.Println("  back                Navigate back")
		fmt.Println("  forward             Navigate forward")
		fmt.Println("  reload              Reload page")
		fmt.Println("  url                 Print URL + title")
		fmt.Println("  scroll <dir> [px]   Scroll")
		fmt.Println("  tabs [n|close]      List/switch/close tabs")
		fmt.Println("  frames              List frames")
		fmt.Println("  frame <n|n.n|url|main>  Switch frame (chain: 0.0)")
		fmt.Println("  upload <sel> <file> Upload file(s)")
		fmt.Println("  text <sel> [--all]   Read visible text content")
		fmt.Println("  source [sel]        Dump HTML (page or element)")
		fmt.Println("  cookie [name|--set k=v|--delete k]")
		fmt.Println("  storage [key|--set k=v|--delete k] [--session]")
		fmt.Println("  drag <sel> <sel>    Drag element to element")
		fmt.Println("  assert <sel> <text> Verify element text")
		fmt.Println("  highlight <sel>     Flash element overlay")
		fmt.Println("  eval <js>           Run JavaScript")
		fmt.Println("  wait <sel|ms>       Wait for element/time")
		fmt.Println("  screenshot [path]   Screenshot")
		fmt.Println("  log [console|network|errors]  Stream events")
		fmt.Println()
		fmt.Println("Global flags:")
		fmt.Println("  --timeout <ms>      Element lookup timeout (default 5000)")
		os.Exit(0)
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", r)
			os.Exit(1)
		}
	}()

	// Parse global flags
	var cmdArgs []string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--timeout" && i+1 < len(os.Args) {
			ms, _ := strconv.Atoi(os.Args[i+1])
			if ms > 0 {
				globalTimeout = time.Duration(ms) * time.Millisecond
			}
			i++
		} else {
			cmdArgs = append(cmdArgs, os.Args[i])
		}
	}
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: no command specified")
		os.Exit(1)
	}
	cmd := cmdArgs[0]
	args := cmdArgs[1:]

	switch cmd {
	case "launch":     cmdLaunch(args)
	case "stop":       cmdStop(args)
	case "ui":         cmdUI(args)
	case "click":      cmdClick(args)
	case "fill":       cmdFill(args)
	case "type":       cmdType(args)
	case "press":      cmdPress(args)
	case "select":     cmdSelect(args)
	case "hover":      cmdHover(args)
	case "navigate":   cmdNavigate(args)
	case "back":       cmdBack(args)
	case "forward":    cmdForward(args)
	case "reload":     cmdReload(args)
	case "url":        cmdURL(args)
	case "scroll":     cmdScroll(args)
	case "tabs":       cmdTabs(args)
	case "frames":     cmdFrames(args)
	case "frame":      cmdFrame(args)
	case "upload":     cmdUpload(args)
	case "text":       cmdText(args)
	case "source":     cmdSource(args)
	case "connect":    cmdConnect(args)
	case "cookie":     cmdCookie(args)
	case "storage":    cmdStorage(args)
	case "drag":       cmdDrag(args)
	case "assert":     cmdAssert(args)
	case "highlight":  cmdHighlight(args)
	case "log":        cmdLog(args)
	case "map":        cmdMap(args)
	case "eval":       cmdEval(args)
	case "wait":       cmdWait(args)
	case "screenshot": cmdScreenshot(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
