//go:build windows

package main

// The tray icon: the agent's face on the PC. Everything real happens in the
// same code the `agent run` command uses; the tray only shows what is going on
// and offers the few actions that make sense from a mouse.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/systray"

	"queueup/internal/agentcfg"
)

// cmdTray runs the agent under a system tray icon. It is the mode the shortcut
// installed by `agent install-autostart` uses.
func cmdTray(args []string) error {
	// The tray wraps cmdRun: same flags, same behaviour, plus an icon. State is
	// passed between them through trayState below.
	statusSink = trayState.set
	go func() {
		trayState.set("Starting")
		if err := cmdRun(args); err != nil {
			trayState.set("Stopped: " + err.Error())
		} else {
			trayState.set("Stopped")
		}
	}()

	systray.Run(trayReady, func() { os.Exit(0) })
	return nil
}

func trayReady() {
	systray.SetTitle("QueueUp")
	systray.SetTooltip("QueueUp agent")
	systray.SetTemplateIcon(trayIcon, trayIcon)

	status := systray.AddMenuItem("Starting", "Current status")
	status.Disable()
	systray.AddSeparator()
	openWeb := systray.AddMenuItem("Open the QueueUp website", "")
	autostart := systray.AddMenuItemCheckbox("Start with Windows", "", autostartInstalled())
	systray.AddSeparator()
	openLog := systray.AddMenuItem("Open the log file", "For debugging")
	openCfg := systray.AddMenuItem("Open the settings folder", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop the agent. Joins cannot run while it is off.")

	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				status.SetTitle(trayState.get())
			case <-openWeb.ClickedCh:
				openBrowser(webURL())
			case <-autostart.ClickedCh:
				// Toggle to the opposite of what the tick currently shows.
				want := !autostart.Checked()
				if err := setAutostart(want); err != nil {
					trayState.set("Couldn't change that setting: " + err.Error())
					break
				}
				if want {
					autostart.Check()
				} else {
					autostart.Uncheck()
				}
			case <-openLog.ClickedCh:
				openFile(logFilePath())
			case <-openCfg.ClickedCh:
				if p, err := agentcfg.DefaultPath(); err == nil {
					openFile(filepath.Dir(p))
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func webURL() string {
	if u := os.Getenv("QUEUEUP_WEB_URL"); u != "" {
		return u
	}
	p, err := agentcfg.DefaultPath()
	if err != nil {
		return ""
	}
	cfg, err := agentcfg.Load(p)
	if err != nil {
		return ""
	}
	if cfg.WebURL != "" {
		return cfg.WebURL
	}
	// Fall back to the relay address; pass --web at pairing time (or set
	// QUEUEUP_WEB_URL) when the site lives elsewhere.
	return strings.TrimSuffix(cfg.RelayURL, "/")
}

func openBrowser(url string) {
	if url == "" {
		return
	}
	_ = exec.Command("cmd", "/c", "start", "", url).Run()
}

func openFile(path string) {
	if path == "" {
		return
	}
	_ = exec.Command("cmd", "/c", "start", "", path).Run()
}

// trayIcon is a tiny generated 16x16 ICO: an orange square with a white Q-ish
// notch. Cosmetic only; replaced with real artwork whenever branding exists.
var trayIcon = buildIco()

func buildIco() []byte {
	// A 16x16, 32-bit uncompressed ICO, built in code so the repo needs no
	// binary asset.
	const w, h = 16, 16
	// BITMAPINFOHEADER(40) + pixels + AND mask
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			// Orange fill, dark notch in the lower right quarter.
			b, g, r, a := byte(0x2a), byte(0x5a), byte(0xd0), byte(0xff)
			if x >= 9 && x <= 12 && y >= 3 && y <= 6 {
				b, g, r = 0x13, 0x11, 0x0f
			}
			// Rounded corners.
			if (x == 0 || x == w-1) && (y == 0 || y == h-1) {
				a = 0
			}
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = b, g, r, a
		}
	}
	andMask := make([]byte, h*4) // 16 bits per row, padded to 4 bytes

	img := make([]byte, 0, 40+len(pixels)+len(andMask))
	hdr := make([]byte, 40)
	putU32 := func(b []byte, off int, v uint32) {
		b[off], b[off+1], b[off+2], b[off+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	}
	putU32(hdr, 0, 40)
	putU32(hdr, 4, w)
	putU32(hdr, 8, h*2) // ICO stores double height (XOR + AND)
	hdr[12], hdr[14] = 1, 32
	img = append(img, hdr...)
	// Rows bottom-up.
	for y := h - 1; y >= 0; y-- {
		img = append(img, pixels[y*w*4:(y+1)*w*4]...)
	}
	img = append(img, andMask...)

	ico := make([]byte, 6+16)
	ico[2] = 1 // type: icon
	ico[4] = 1 // count
	ico[6] = w
	ico[7] = h
	ico[12] = 32
	putU32(ico, 14, uint32(len(img)))
	putU32(ico, 18, uint32(len(ico)))
	return append(ico, img...)
}

// trayStatus is shared between the run loop and the tray.
type trayStatus struct {
	ch chan string
	v  string
}

var trayState = func() *trayStatus {
	t := &trayStatus{ch: make(chan string, 16), v: "Starting"}
	go func() {
		for s := range t.ch {
			t.v = s
		}
	}()
	return t
}()

func (t *trayStatus) set(s string) {
	select {
	case t.ch <- s:
	default:
	}
}

func (t *trayStatus) get() string { return t.v }

var _ = fmt.Sprintf
var _ = context.Background
