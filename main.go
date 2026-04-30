package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
)

const (
	cdpFile   = "/tmp/web-control-cdp.txt"
	frameFile = "/tmp/web-control-frame.txt"
)

// ── Element extraction JS (same as web.py) ────────────────────
const extractJS = `() => {
	const seen = new Set();
	const results = [];
	function walk(root) {
		const els = root.querySelectorAll([
			'a[href]','button','input','select','textarea',
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
		fmt.Fprintln(os.Stderr, "ERROR: no browser running. Use 'web launch' first.")
		os.Exit(1)
	}
	wsURL := strings.TrimSpace(string(data))
	browser := rod.New().ControlURL(wsURL).MustConnect()
	return browser
}

func getPage(browser *rod.Browser) *rod.Page {
	pages := browser.MustPages()
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
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: web navigate <url>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	page.MustNavigate(args[0]).MustWaitDOMStable()
	info := page.MustInfo()
	fmt.Printf("-> %s\n   %s\n", info.URL, info.Title)
}

func cmdClick(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: web click <selector>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	if args[0] == "--text" && len(args) > 1 {
		frame.MustElementR("*", args[1]).MustClick()
		fmt.Printf("Clicked text: %s\n", args[1])
	} else if args[0] == "--xy" && len(args) > 2 {
		x, _ := strconv.ParseFloat(args[1], 64)
		y, _ := strconv.ParseFloat(args[2], 64)
		page.Mouse.MustMoveTo(x, y)
		page.Mouse.MustDown("left")
		page.Mouse.MustUp("left")
		fmt.Printf("Clicked (%s, %s)\n", args[1], args[2])
	} else {
		frame.MustElement(args[0]).MustClick()
		fmt.Printf("Clicked: %s\n", args[0])
	}
}

func cmdFill(args []string) {
	if len(args) < 2 { fmt.Fprintln(os.Stderr, "Usage: web fill <selector> <text>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	el := frame.MustElement(args[0])
	el.MustSelectAllText().MustInput(strings.Join(args[1:], " "))
	fmt.Printf("Filled %s\n", args[0])
}

func cmdType(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: web type <text>"); os.Exit(1) }
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
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: web eval <js>"); os.Exit(1) }
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
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: web wait <selector|ms>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)

	if ms, err := strconv.Atoi(args[0]); err == nil {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		fmt.Printf("Waited %dms\n", ms)
	} else {
		frame.MustElement(args[0])
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
	el := frame.MustElement(args[0])
	el.MustSelect(args[1:]...)
	fmt.Printf("Selected: %s\n", strings.Join(args[1:], ", "))
}

func cmdHover(args []string) {
	if len(args) == 0 { fmt.Fprintln(os.Stderr, "Usage: wdb hover <selector>"); os.Exit(1) }
	browser := connectBrowser()
	page := getPage(browser)
	frame := getFrame(page)
	if args[0] == "--text" && len(args) > 1 {
		frame.MustElementR("*", args[1]).MustHover()
		fmt.Printf("Hovered text: %s\n", args[1])
	} else {
		frame.MustElement(args[0]).MustHover()
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
	el := frame.MustElement(args[0])
	el.MustSetFiles(args[1:]...)
	fmt.Printf("Uploaded %d file(s) to %s\n", len(args)-1, args[0])
}

// ── Main ──────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		fmt.Println("wdb - Web Debug Bridge (browser control CLI)")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  launch [url]        Launch browser")
		fmt.Println("  stop                Close browser")
		fmt.Println("  ui [--all|--json|--filter x|--limit n]")
		fmt.Println("  click <sel>         Click (or --text/--xy)")
		fmt.Println("  fill <sel> <text>   Clear + type")
		fmt.Println("  type <text>         Type into focused")
		fmt.Println("  press <key>         Press key (Enter, Tab, Escape...)")
		fmt.Println("  select <sel> <opt>  Select dropdown option by text")
		fmt.Println("  hover <sel>         Hover (or --text)")
		fmt.Println("  navigate <url>      Go to URL")
		fmt.Println("  back                Navigate back")
		fmt.Println("  forward             Navigate forward")
		fmt.Println("  reload              Reload page")
		fmt.Println("  url                 Print URL + title")
		fmt.Println("  scroll <dir> [px]   Scroll")
		fmt.Println("  tabs [n|close]      List/switch/close tabs")
		fmt.Println("  frames              List frames")
		fmt.Println("  frame <n|n.n|url|main>  Switch frame (chain: 0.0)")
		fmt.Println("  upload <sel> <file> Upload file(s)")
		fmt.Println("  eval <js>           Run JavaScript")
		fmt.Println("  wait <sel|ms>       Wait for element/time")
		fmt.Println("  screenshot [path]   Screenshot")
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

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
