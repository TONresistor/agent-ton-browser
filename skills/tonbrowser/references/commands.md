# agent-tonbrowser CLI Reference

Binary: `agent-tonbrowser` (locate via `which agent-tonbrowser` or `find ~ -name agent-tonbrowser -type f -executable 2>/dev/null | head -1`)

## Global Flags

| Flag | Default | Env | Description |
|------|---------|-----|-------------|
| `--cdp-port` | 9222 | `AGENT_TONBROWSER_CDP_PORT` | CDP port |
| `--tab` | -1 | `AGENT_TONBROWSER_TAB` | Target index (-1=auto, 0=main UI, 1+=.ton tabs) |
| `--json` | false | `AGENT_TONBROWSER_JSON` | JSON output |
| `--timeout` | 30000 | `AGENT_TONBROWSER_TIMEOUT` | Action timeout (ms) |

## Target Selection (--tab)

Tonnet Browser exposes multiple CDP targets:
- **Tab 0 (or -1)**: Main renderer : the browser UI (address bar, tab bar, status bar)
- **Tab 1+**: WebContentsViews : the actual .ton page content loaded in each tab

**Rule**: Use `--tab 0` to interact with the browser UI (fill address bar, click nav buttons). Use `--tab 1` (or higher) to read/interact with the .ton page content.

After navigation, always re-check target order:
```bash
curl -s http://127.0.0.1:9222/json/list | python3 -c "
import sys,json
for i,t in enumerate(json.load(sys.stdin)):
    if not t.get('url','').startswith('devtools://'):
        print(f'[{i}] {t.get(\"title\",\"\")} : {t.get(\"url\",\"\")[:80]}')
"
```

## Commands

### Browser Lifecycle

```bash
# Download and install Tonnet Browser (latest or specific version)
agent-tonbrowser install [--version 1.4.2]

# Launch with CDP enabled
agent-tonbrowser launch [--port 9222]

# Close the running browser
agent-tonbrowser close

# Show browser + proxy status
agent-tonbrowser status
```

### Connection

```bash
# Verify CDP connection
agent-tonbrowser connect [port]
```

### Navigation (use --tab 0 for browser UI)

```bash
# Fill address bar and press Enter to navigate
agent-tonbrowser --tab 0 fill "input" "site.ton"
agent-tonbrowser --tab 0 press Enter

# Go back/forward/reload (affects active tab)
agent-tonbrowser --tab 0 eval 'document.querySelector("button[aria-label=\"Back\"]")?.click()'
```

### Observation

```bash
# Screenshot a target
agent-tonbrowser --tab 1 screenshot output.png
agent-tonbrowser --tab 0 screenshot browser-ui.png

# Full page screenshot
agent-tonbrowser --tab 1 screenshot -f full.png

# Accessibility tree snapshot
agent-tonbrowser --tab 1 snapshot [-i] [-c] [-d 3]
#   -i  interactive elements only
#   -c  compact (remove structural nodes)
#   -d  max depth

# Get page info
agent-tonbrowser --tab 1 get title
agent-tonbrowser --tab 1 get url
```

### Interaction (use --tab matching the target)

```bash
# Click element by CSS selector
agent-tonbrowser --tab 1 click "button.search"

# Fill input (clears first via Ctrl+A)
agent-tonbrowser --tab 1 fill "input[type=search]" "query"

# Type without clearing
agent-tonbrowser --tab 1 type "input" "text"

# Press key (Enter, Tab, Escape, Backspace, Delete, ArrowUp/Down/Left/Right, Home, End, PageUp, PageDown, Space)
agent-tonbrowser --tab 1 press Enter

# Scroll (up/down/left/right, default 300px)
agent-tonbrowser --tab 1 scroll down --pixels 500

# Wait for element, load, or time
agent-tonbrowser --tab 1 wait ".content"
agent-tonbrowser --tab 1 wait load
agent-tonbrowser --tab 1 wait 2000
```

### JavaScript

```bash
# Execute JS and get result
agent-tonbrowser --tab 1 eval 'document.title'
agent-tonbrowser --tab 1 eval 'document.body.innerText.substring(0, 1000)'
agent-tonbrowser --tab 1 eval 'document.querySelectorAll("a").length'

# Complex expressions (use IIFE to avoid redeclaration errors)
agent-tonbrowser --tab 1 eval '(() => {
  const links = Array.from(document.querySelectorAll("a"));
  return links.map(a => a.href + " | " + a.textContent.trim()).join("\n");
})()'
```

### Tab Listing

```bash
# List all page targets
agent-tonbrowser tab
```

## Known Behaviors

- **Snapshot AX tree is minimal on Electron renderers**: Use `eval 'document.body.innerText'` for text content instead.
- **Use IIFE for complex eval**: Wrap in `(() => { ... })()` to avoid `const`/`let` redeclaration errors across invocations.
- **Wait after navigation**: Allow 5-10s for .ton pages to load via TON proxy before reading content.
- **Screenshot timeout**: If screenshot times out, retry once : the raw CDP executor is reliable but Electron can be slow on first capture.
