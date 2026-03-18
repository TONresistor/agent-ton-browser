package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"

	"github.com/TONresistor/agent-tonbrowser/internal/browser"
	"github.com/TONresistor/agent-tonbrowser/internal/config"
	"github.com/TONresistor/agent-tonbrowser/internal/daemon"
	"github.com/TONresistor/agent-tonbrowser/internal/ton"
)

var version = "dev" // injected via ldflags

// ExitCode constants for semantic error reporting.
const (
	ExitOK           = 0
	ExitGeneral      = 1
	ExitNotFound     = 2
	ExitDNSFail      = 3
	ExitProxyDown    = 4
	ExitAlreadyExists = 5
)

type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string { return e.msg }
func (e *cliError) ExitCode() int { return e.code }

func exitErr(code int, format string, args ...any) error {
	return &cliError{code: code, msg: fmt.Sprintf(format, args...)}
}

// Globals holds flags available on all commands.
type Globals struct {
	CDPPort int    `help:"CDP port to connect to" default:"9222" env:"AGENT_TONBROWSER_CDP_PORT"`
	Tab     int    `help:"Target tab index (0=main UI, 1=first .ton tab, etc.)" default:"-1" env:"AGENT_TONBROWSER_TAB"`
	TabURL  string `help:"Select tab by URL pattern (e.g. '*.ton')" default:"" env:"AGENT_TONBROWSER_TAB_URL"`
	JSON    bool   `help:"Output in JSON format" env:"AGENT_TONBROWSER_JSON"`
	Debug   bool   `help:"Debug output" env:"AGENT_TONBROWSER_DEBUG"`
	Timeout int    `help:"Action timeout in milliseconds" default:"30000" env:"AGENT_TONBROWSER_TIMEOUT"`
}

// resolveTab returns the effective tab index: if --tab-url is set, connect to the matching
// tab and return -1 (daemon's current tab); otherwise return globals.Tab.
func resolveTab(globals *Globals) int {
	if globals.TabURL != "" {
		if err := daemon.EnsureDaemon(); err == nil {
			if err := daemon.ConnectByURL(globals.CDPPort, globals.TabURL); err == nil {
				return -1
			}
		}
	}
	return globals.Tab
}

// CLI is the root kong struct.
type CLI struct {
	Globals

	Daemon     DaemonCmd     `cmd:"" help:"Manage the background CDP daemon"`
	Install    InstallCmd    `cmd:"" help:"Download and install Tonnet Browser"`
	Launch     LaunchCmd     `cmd:"" help:"Launch Tonnet Browser with CDP enabled"`
	Close      CloseCmd      `cmd:"" help:"Close the running Tonnet Browser"`
	Status     StatusCmd     `cmd:"" help:"Show browser and proxy status"`
	Connect    ConnectCmd    `cmd:"" help:"Connect daemon to a running browser via CDP port (explicit port/tab selection)"`
	Goto       GotoCmd       `cmd:"" help:"Navigate to a .ton URL" aliases:"open,navigate"`
	Back       BackCmd       `cmd:"" help:"Go back in history"`
	Forward    ForwardCmd    `cmd:"" help:"Go forward in history"`
	Reload     ReloadCmd     `cmd:"" help:"Reload current page"`
	Snapshot   SnapshotCmd   `cmd:"" help:"Get accessibility tree snapshot"`
	Screenshot ScreenshotCmd `cmd:"" help:"Take a screenshot"`
	Get        GetCmd        `cmd:"" help:"Get page info (url, title)"`
	Tab        TabCmd        `cmd:"" help:"List or switch tabs"`
	Click      ClickCmd      `cmd:"" help:"Click an element"`
	Fill       FillCmd       `cmd:"" help:"Clear and fill an input"`
	Type       TypeCmd       `cmd:"" help:"Type text without clearing"`
	Press      PressCmd      `cmd:"" help:"Press a key (Enter, Tab, Escape, etc.)"`
	Scroll     ScrollCmd     `cmd:"" help:"Scroll the page"`
	Wait       WaitCmd       `cmd:"" help:"Wait for element, time, or page load"`
	Eval       EvalCmd       `cmd:"" help:"Execute JavaScript"`
	DNS        DNSCmd        `cmd:"" help:"TON DNS utilities"`
	Version    VersionCmd    `cmd:"" help:"Show version"`
}

// printJSON marshals v to indented JSON and prints it.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

// ─── Daemon management ────────────────────────────────────────────────────────

// DaemonCmd manages the background CDP daemon process.
type DaemonCmd struct {
	Start  DaemonStartCmd  `cmd:"" help:"Start the daemon (called internally by EnsureDaemon)"`
	Stop   DaemonStopCmd   `cmd:"" help:"Stop the running daemon"`
	Status DaemonStatusCmd `cmd:"" help:"Show daemon status"`
}

// DaemonStartCmd is the daemon entry point — called by StartDaemon() via fork.
type DaemonStartCmd struct{}

func (c *DaemonStartCmd) Run(globals *Globals) error {
	return daemon.NewServer(0).Run()
}

// DaemonStopCmd stops the running daemon.
type DaemonStopCmd struct{}

func (c *DaemonStopCmd) Run(globals *Globals) error {
	if err := daemon.StopDaemon(); err != nil {
		return fmt.Errorf("stop daemon: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"stopped": true})
	}
	fmt.Println("Daemon stopped.")
	return nil
}

// DaemonStatusCmd shows the daemon's current status.
type DaemonStatusCmd struct{}

func (c *DaemonStatusCmd) Run(globals *Globals) error {
	status, err := daemon.DaemonStatus()
	if err != nil {
		return fmt.Errorf("daemon status: %w", err)
	}
	if globals.JSON {
		return printJSON(status)
	}
	fmt.Printf("PID:       %d\n", status.PID)
	fmt.Printf("Connected: %v\n", status.Connected)
	fmt.Printf("Port:      %d\n", status.Port)
	fmt.Printf("Tab:       %d\n", status.Tab)
	return nil
}

// ─── Browser lifecycle ────────────────────────────────────────────────────────

// InstallCmd downloads and installs Tonnet Browser.
type InstallCmd struct {
	Version string `help:"Version to install (default: latest)" optional:""`
}

func (c *InstallCmd) Run(globals *Globals) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(globals.Timeout)*time.Millisecond)
	defer cancel()

	ver := c.Version
	if ver == "" {
		fmt.Println("Fetching latest version...")
		var err error
		ver, err = browser.LatestVersion(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch latest version: %w", err)
		}
	}

	fmt.Printf("Installing Tonnet Browser %s...\n", ver)
	path, err := browser.Download(ctx, ver)
	if err != nil {
		return fmt.Errorf("failed to install: %w", err)
	}

	if globals.JSON {
		return printJSON(map[string]string{"version": ver, "path": path})
	}
	fmt.Printf("Installed: %s\n", path)
	return nil
}

// LaunchCmd launches Tonnet Browser with CDP enabled.
type LaunchCmd struct {
	Port    int    `help:"CDP port (overrides --cdp-port)" default:"0" optional:""`
	Version string `help:"Version to launch (default: any installed)" optional:""`
}

func (c *LaunchCmd) Run(globals *Globals) error {
	port := globals.CDPPort
	if c.Port != 0 {
		port = c.Port
	}

	// Launch needs more time than typical actions — use at least 60s
	timeout := time.Duration(globals.Timeout) * time.Millisecond
	if timeout < 60*time.Second {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exePath := browser.DetectInstalled(c.Version)
	if exePath == "" {
		exePath = browser.DetectSystem()
	}
	if exePath == "" {
		return fmt.Errorf("Tonnet Browser not found — run 'install' first")
	}

	ver := c.Version
	if ver == "" {
		ver = "unknown"
	}

	fmt.Printf("Launching %s on port %d...\n", exePath, port)
	wsURL, err := browser.Launch(ctx, exePath, port, ver)
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	if globals.JSON {
		return printJSON(map[string]any{"ws_url": wsURL, "port": port})
	}
	fmt.Printf("Browser launched. CDP: %s\n", wsURL)
	return nil
}

// CloseCmd closes the running Tonnet Browser and stops the daemon.
type CloseCmd struct{}

func (c *CloseCmd) Run(globals *Globals) error {
	if err := browser.Close(); err != nil {
		return fmt.Errorf("failed to close browser: %w", err)
	}
	// Best-effort: stop daemon if it was running
	_ = daemon.StopDaemon()
	if globals.JSON {
		return printJSON(map[string]bool{"closed": true})
	}
	fmt.Println("Browser closed.")
	return nil
}

// StatusCmd shows browser and proxy status.
type StatusCmd struct{}

func (c *StatusCmd) Run(globals *Globals) error {
	state, err := config.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	browserRunning := state != nil && state.IsRunning()
	proxyRunning, proxyPort, _ := ton.ProxyStatus()

	if globals.JSON {
		type statusOut struct {
			BrowserRunning bool   `json:"browser_running"`
			ProxyRunning   bool   `json:"proxy_running"`
			ProxyPort      int    `json:"proxy_port"`
			CDPPort        int    `json:"cdp_port,omitempty"`
			Version        string `json:"version,omitempty"`
			PID            int    `json:"pid,omitempty"`
		}
		out := statusOut{
			BrowserRunning: browserRunning,
			ProxyRunning:   proxyRunning,
			ProxyPort:      proxyPort,
		}
		if state != nil {
			out.CDPPort = state.CDPPort
			out.Version = state.Version
			out.PID = state.PID
		}
		return printJSON(out)
	}

	fmt.Print("Browser: ")
	if browserRunning {
		fmt.Printf("running (PID %d, port %d, version %s)\n", state.PID, state.CDPPort, state.Version)
	} else {
		fmt.Println("not running")
	}
	fmt.Print("Proxy:   ")
	if proxyRunning {
		fmt.Printf("running (port %d)\n", proxyPort)
	} else {
		fmt.Println("not running")
	}
	return nil
}

// ─── Connection ───────────────────────────────────────────────────────────────

// ConnectCmd tells the daemon to connect to a running browser via CDP.
type ConnectCmd struct {
	Port *int `arg:"" optional:"" help:"CDP port (default: --cdp-port)"`
}

func (c *ConnectCmd) Run(globals *Globals) error {
	port := globals.CDPPort
	if c.Port != nil {
		port = *c.Port
	}

	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Connect(port, globals.Tab); err != nil {
		return fmt.Errorf("cannot connect to browser on port %d: %w", port, err)
	}

	if globals.JSON {
		return printJSON(map[string]any{"connected": true, "port": port})
	}
	fmt.Printf("Connected to browser on port %d\n", port)
	return nil
}

// ─── Navigation ───────────────────────────────────────────────────────────────

// GotoCmd navigates to a .ton URL.
type GotoCmd struct {
	URL string `arg:"" help:"The .ton URL to navigate to"`
}

func (c *GotoCmd) Run(globals *Globals) error {
	if !ton.IsTonURL(c.URL) {
		return exitErr(ExitGeneral, "not a .ton URL: %q", c.URL)
	}

	proxyRunning, _, _ := ton.ProxyStatus()
	if !proxyRunning {
		return exitErr(ExitProxyDown, "tonnet-proxy is not running — start Tonnet Browser first")
	}

	normalized := ton.NormalizeURL(c.URL)
	tab := resolveTab(globals)

	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Navigate(tab, normalized); err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}
	currentURL, _ := daemon.GetURL(tab)
	title, _ := daemon.GetTitle(tab)

	if globals.JSON {
		return printJSON(map[string]string{"url": currentURL, "title": title})
	}
	fmt.Printf("URL:   %s\n", currentURL)
	fmt.Printf("Title: %s\n", title)
	return nil
}

// BackCmd goes back in history.
type BackCmd struct{}

func (c *BackCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Back(tab); err != nil {
		return fmt.Errorf("failed to go back: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Went back.")
	return nil
}

// ForwardCmd goes forward in history.
type ForwardCmd struct{}

func (c *ForwardCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Forward(tab); err != nil {
		return fmt.Errorf("failed to go forward: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Went forward.")
	return nil
}

// ReloadCmd reloads the current page.
type ReloadCmd struct{}

func (c *ReloadCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Reload(tab); err != nil {
		return fmt.Errorf("failed to reload: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Reloaded.")
	return nil
}

// ─── Observation ──────────────────────────────────────────────────────────────

// SnapshotCmd gets the accessibility tree snapshot.
type SnapshotCmd struct {
	Interactive bool `short:"i" help:"Show only interactive elements"`
	Compact     bool `short:"c" help:"Remove empty structural nodes"`
	Depth       int  `short:"d" help:"Maximum tree depth (0 = unlimited)" default:"0"`
}

func (c *SnapshotCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	text, err := daemon.Snapshot(tab, c.Interactive, c.Compact, c.Depth)
	if err != nil {
		return fmt.Errorf("failed to snapshot: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]string{"snapshot": text})
	}
	fmt.Print(text)
	return nil
}

// ScreenshotCmd takes a screenshot.
type ScreenshotCmd struct {
	Path     string `arg:"" optional:"" help:"Output file path (auto-generated if empty)"`
	Full     bool   `short:"f" help:"Full page screenshot"`
	Annotate bool   `short:"a" help:"Add numbered labels on interactive elements"`
}

func (c *ScreenshotCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if c.Annotate {
		outPath, labels, err := daemon.AnnotatedScreenshot(tab, c.Path)
		if err != nil {
			return fmt.Errorf("failed to take annotated screenshot: %w", err)
		}
		if globals.JSON {
			return printJSON(map[string]any{"path": outPath, "labels": labels})
		}
		fmt.Printf("Screenshot saved: %s\n", outPath)
		var indented bytes.Buffer
		json.Indent(&indented, labels, "", "  ")
		fmt.Println(indented.String())
		return nil
	}
	outPath, err := daemon.Screenshot(tab, c.Path, c.Full)
	if err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]string{"path": outPath})
	}
	fmt.Printf("Screenshot saved: %s\n", outPath)
	return nil
}

// GetCmd gets the current page URL or title.
type GetCmd struct {
	What string `arg:"" help:"What to get: url, title" enum:"url,title"`
}

func (c *GetCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	var value string
	var err error
	switch c.What {
	case "url":
		value, err = daemon.GetURL(tab)
	case "title":
		value, err = daemon.GetTitle(tab)
	}
	if err != nil {
		return fmt.Errorf("failed to get %s: %w", c.What, err)
	}
	if globals.JSON {
		return printJSON(map[string]string{c.What: value})
	}
	fmt.Println(value)
	return nil
}

// TabCmd lists all tabs or switches to a tab by index.
type TabCmd struct {
	Index *int `arg:"" optional:"" help:"Tab index to switch to (omit to list all tabs)"`
}

func (c *TabCmd) Run(globals *Globals) error {
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if c.Index != nil {
		if err := daemon.SwitchTab(*c.Index); err != nil {
			return fmt.Errorf("failed to switch tab: %w", err)
		}
		if globals.JSON {
			return printJSON(map[string]any{"switched_to": *c.Index})
		}
		fmt.Printf("Switched to tab %d\n", *c.Index)
		return nil
	}

	tabs, err := daemon.ListTabs()
	if err != nil {
		return fmt.Errorf("failed to list tabs: %w", err)
	}
	if globals.JSON {
		return printJSON(tabs)
	}
	for _, tab := range tabs {
		fmt.Printf("[%d] %s — %s\n", tab.Index, tab.Title, tab.URL)
	}
	return nil
}

// ─── Interaction ──────────────────────────────────────────────────────────────

// ClickCmd clicks an element.
type ClickCmd struct {
	Selector string `arg:"" help:"CSS selector, XPath expression, or @eN ref"`
}

func (c *ClickCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Click(tab, c.Selector); err != nil {
		return fmt.Errorf("failed to click: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Clicked.")
	return nil
}

// FillCmd clears an input and types new text.
type FillCmd struct {
	Selector string `arg:"" help:"CSS selector, XPath expression, or @eN ref"`
	Text     string `arg:"" help:"Text to fill"`
}

func (c *FillCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Fill(tab, c.Selector, c.Text); err != nil {
		return fmt.Errorf("failed to fill: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Filled.")
	return nil
}

// TypeCmd types text into an element without clearing it first.
type TypeCmd struct {
	Selector string `arg:"" help:"CSS selector, XPath expression, or @eN ref"`
	Text     string `arg:"" help:"Text to type"`
}

func (c *TypeCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.TypeText(tab, c.Selector, c.Text); err != nil {
		return fmt.Errorf("failed to type: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Typed.")
	return nil
}

// PressCmd presses a key on the focused element.
type PressCmd struct {
	Key string `arg:"" help:"Key to press: Enter, Tab, Escape, or a single character"`
}

func (c *PressCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Press(tab, c.Key); err != nil {
		return fmt.Errorf("failed to press key: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Printf("Pressed %q\n", c.Key)
	return nil
}

// ScrollCmd scrolls the page in a direction.
type ScrollCmd struct {
	Direction string `arg:"" help:"Direction to scroll: up, down, left, right" enum:"up,down,left,right"`
	Pixels    int    `help:"Number of pixels to scroll" default:"300"`
}

func (c *ScrollCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Scroll(tab, c.Direction, c.Pixels); err != nil {
		return fmt.Errorf("failed to scroll: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]any{"direction": c.Direction, "pixels": c.Pixels})
	}
	fmt.Printf("Scrolled %s by %d px\n", c.Direction, c.Pixels)
	return nil
}

// WaitCmd waits for a condition before continuing.
type WaitCmd struct {
	Target string `arg:"" help:"CSS selector, milliseconds, 'load', or 'network'"`
}

func (c *WaitCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	if err := daemon.Wait(tab, c.Target); err != nil {
		return fmt.Errorf("failed to wait: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("Done waiting.")
	return nil
}

// EvalCmd executes JavaScript in the page context.
type EvalCmd struct {
	Script string `arg:"" help:"JavaScript to execute"`
}

func (c *EvalCmd) Run(globals *Globals) error {
	tab := resolveTab(globals)
	if err := daemon.EnsureDaemon(); err != nil {
		return err
	}
	result, err := daemon.Eval(tab, c.Script)
	if err != nil {
		return fmt.Errorf("failed to eval: %w", err)
	}
	if globals.JSON {
		return printJSON(map[string]string{"result": result})
	}
	fmt.Println(result)
	return nil
}

// ─── DNS ──────────────────────────────────────────────────────────────────────

// DNSCmd groups TON DNS subcommands.
type DNSCmd struct {
	Resolve DNSResolveCmd `cmd:"" help:"Resolve a .ton domain"`
}

// DNSResolveCmd resolves a .ton domain via the TON DNS system.
type DNSResolveCmd struct {
	Domain string `arg:"" help:"The .ton domain to resolve (e.g. foundation.ton)"`
}

func (c *DNSResolveCmd) Run(globals *Globals) error {
	result, err := daemon.DNSResolve(c.Domain)
	if err != nil {
		return exitErr(ExitDNSFail, "dns resolve: %v", err)
	}
	if globals.JSON {
		fmt.Println(string(result))
		return nil
	}
	var indented bytes.Buffer
	json.Indent(&indented, result, "", "  ")
	fmt.Println(indented.String())
	return nil
}

// ─── Version ──────────────────────────────────────────────────────────────────

// VersionCmd prints the binary version.
type VersionCmd struct{}

func (c *VersionCmd) Run(globals *Globals) error {
	if globals.JSON {
		return printJSON(map[string]string{"version": version})
	}
	fmt.Printf("agent-tonbrowser %s\n", version)
	return nil
}

// ─── Entry point ──────────────────────────────────────────────────────────────

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("agent-tonbrowser"),
		kong.Description("CLI agent for Tonnet Browser automation via CDP"),
		kong.UsageOnError(),
	)
	err := ctx.Run(&cli.Globals)
	if err != nil {
		var cliErr *cliError
		if errors.As(err, &cliErr) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(cliErr.ExitCode())
		}
		ctx.FatalIfErrorf(err)
	}
}
