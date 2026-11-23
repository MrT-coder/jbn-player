// Island spike: open a small WebView2 window (optionally with an HTML file
// given as argument), report nothing itself, close after a timeout.
// Usage: island-spike [page.html]  (env SPIKE_DATA = unique WebView2 data dir)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jchv/go-webview2"
)

const defaultPage = `<!doctype html><meta charset=utf-8><body style="font:14px sans-serif;background:#16161a;color:#eee;margin:16px">
<h3>jbn-player island spike</h3><p>Small window, created on open, destroyed on close.</p></body>`

func main() {
	data := os.Getenv("SPIKE_DATA")
	if data == "" {
		data = filepath.Join(os.TempDir(), "jbn-island-spike-data")
	}
	page := defaultPage
	if len(os.Args) > 1 {
		b, err := os.ReadFile(os.Args[1])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		page = string(b)
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  data,
		WindowOptions: webview2.WindowOptions{
			Title: "jbn-player", Width: 420, Height: 620, Center: true,
		},
	})
	if w == nil {
		fmt.Println("webview2 init failed")
		os.Exit(1)
	}
	defer w.Destroy()
	w.SetHtml(page)
	go func() {
		time.Sleep(30 * time.Second)
		w.Dispatch(w.Terminate)
	}()
	w.Run()
}
