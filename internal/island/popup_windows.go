//go:build windows

package island

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The island behaves like a tray popup (think G-Helper): no title bar, no
// taskbar entry, opens next to the tray icon, closes when it loses focus.

const (
	popupW      = 400
	popupH      = 580
	popupMargin = 12
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")

	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pGetWindowLongPtr    = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtr    = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos        = user32.NewProc("SetWindowPos")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	pGetAncestor         = user32.NewProc("GetAncestor")
	pMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	pAllowSetForeground  = user32.NewProc("AllowSetForegroundWindow")
	pDwmSetWindowAttrib  = dwmapi.NewProc("DwmSetWindowAttribute")
	pEnumWindows         = user32.NewProc("EnumWindows")
	pGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	pGetClassName        = user32.NewProc("GetClassNameW")
	pPostMessage         = user32.NewProc("PostMessageW")
	pSendMessage         = user32.NewProc("SendMessageW")
	pReleaseCapture      = user32.NewProc("ReleaseCapture")

	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	pOpenProcess        = kernel32.NewProc("OpenProcess")
	pCloseHandle        = kernel32.NewProc("CloseHandle")
	pQueryFullImageName = kernel32.NewProc("QueryFullProcessImageNameW")
)

type point struct{ x, y int32 }
type rect struct{ l, t, r, b int32 }
type monitorInfo struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

// CursorPos returns the screen position of the mouse.
func CursorPos() (x, y int) {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.x), int(p.y)
}

// AllowForeground lets process pid take the foreground. The tray process calls
// it right after a click so the new island window can come to the front.
func AllowForeground(pid int) { pAllowSetForeground.Call(uintptr(pid)) }

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// placePopup returns the top-left corner of a w x h popup in the bottom-right
// corner of the work area wa (just above the taskbar, where the tray lives),
// so it always opens in the same place. It can be dragged from there.
func placePopup(wa rect, w, h int) (int, int) {
	l, t, r, b := int(wa.l), int(wa.t), int(wa.r), int(wa.b)
	return clamp(r-w-popupMargin, l+popupMargin, r-w-popupMargin), clamp(b-h-popupMargin, t+popupMargin, b-h-popupMargin)
}

func workAreaAt(x, y int) rect {
	// MONITOR_DEFAULTTONEAREST = 2; the point is passed by value (packed into one register).
	pt := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32
	h, _, _ := pMonitorFromPoint.Call(pt, 2)
	mi := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := pGetMonitorInfo.Call(h, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return rect{0, 0, 1920, 1080}
	}
	return mi.work
}

// stylePopup turns the webview's top-level window into the borderless popup
// and puts it in place. It must run before the message loop starts.
func stylePopup(hwnd uintptr) {
	const (
		wsPopup, wsVisible, wsClip = 0x80000000, 0x10000000, 0x06000000
		exToolWindow, exTopmost    = 0x00000080, 0x00000008
		swpFrameChanged, swpShow   = 0x0020, 0x0040
	)
	gwlStyle, gwlExStyle := -16, -20
	pSetWindowLongPtr.Call(hwnd, uintptr(gwlStyle), wsPopup|wsVisible|wsClip)
	ex, _, _ := pGetWindowLongPtr.Call(hwnd, uintptr(gwlExStyle))
	pSetWindowLongPtr.Call(hwnd, uintptr(gwlExStyle), ex|exToolWindow|exTopmost)

	// The monitor where the mouse is: that is where the user just clicked the tray icon.
	cx, cy := CursorPos()
	x, y := placePopup(workAreaAt(cx, cy), popupW, popupH)

	hwndTopmost := -1
	pSetWindowPos.Call(hwnd, uintptr(hwndTopmost), uintptr(x), uintptr(y), popupW, popupH, swpFrameChanged|swpShow)

	// Windows 11: rounded corners (ignored by older Windows).
	round := int32(2) // DWMWCP_ROUND
	pDwmSetWindowAttrib.Call(hwnd, 33, uintptr(unsafe.Pointer(&round)), 4)
	pSetForegroundWindow.Call(hwnd)
}

// dragWindow starts moving the popup with the mouse, as if its (missing) title
// bar had been grabbed. The page calls it on mouse-down on its grip bar.
func dragWindow(hwnd uintptr) {
	const wmNCLButtonDown, htCaption = 0x00A1, 2
	pReleaseCapture.Call()
	pSendMessage.Call(hwnd, wmNCLButtonDown, htCaption, 0)
}

func windowPID(h uintptr) uint32 {
	var pid uint32
	pGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func processName(pid uint32) string {
	const processQueryLimitedInformation = 0x1000
	h, _, _ := pOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)
	buf := make([]uint16, 520)
	n := uint32(len(buf))
	if r, _, _ := pQueryFullImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(syscall.UTF16ToString(buf[:n])))
}

// Close asks the popup process pid to close its window (WM_CLOSE), which lets
// WebView2 shut down cleanly. It reports whether a window was found.
func Close(pid int) bool {
	found := false
	cb := syscall.NewCallback(func(h, _ uintptr) uintptr {
		if windowPID(h) != uint32(pid) {
			return 1
		}
		buf := make([]uint16, 64)
		pGetClassName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if syscall.UTF16ToString(buf) == "webview" { // the window class go-webview2 registers
			pPostMessage.Call(h, 0x0010, 0, 0) // WM_CLOSE
			found = true
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	return found
}

// watchFocus calls closeFn once the popup has been in the foreground and then
// lost it to something else (the user clicked elsewhere). These do not count
// as "elsewhere": windows owned by the popup (a dropdown) and WebView2's own
// helper windows, which can grab the foreground for a moment. Losing it must
// last two polls in a row. It never fires if the popup never had the
// foreground, so a refused focus cannot make the window vanish.
func watchFocus(hwnd uintptr, closeFn func()) {
	go func() {
		const gaRootOwner = 3
		armed, away := false, 0
		for {
			time.Sleep(150 * time.Millisecond)
			fg, _, _ := pGetForegroundWindow.Call()
			if fg == 0 {
				continue
			}
			owner, _, _ := pGetAncestor.Call(fg, gaRootOwner)
			if fg == hwnd || owner == hwnd {
				armed, away = true, 0
				continue
			}
			if processName(windowPID(fg)) == "msedgewebview2.exe" {
				continue
			}
			if armed {
				away++
				if away >= 2 {
					closeFn()
					return
				}
			}
		}
	}()
}

var (
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx      = user32.NewProc("CallNextHookEx")
	pSetClassLongPtr     = user32.NewProc("SetClassLongPtrW")
	pGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
)

// createStruct is CREATESTRUCTW (64-bit layout; Go inserts the same padding).
type createStruct struct {
	createParams, instance, menu, parent uintptr
	cy, cx, y, x                         int32
	style                                int32
	name, class                          uintptr
	exStyle                              uint32
}

type cbtCreateWnd struct {
	cs          *createStruct
	insertAfter uintptr
}

// placeAtCreation makes the popup's window appear in its final place. go-webview2
// creates its window visible at CW_USEDEFAULT (near the top-left) and only then
// loads WebView2, which takes a few hundred milliseconds, so the popup used to
// show up on the left and then jump to the corner. A creation hook on this
// thread changes the window's position, frame and background BEFORE it exists
// on screen. Call the returned function once the window has been created.
// The thread must stay locked to the same OS thread (see Run).
func placeAtCreation() (unhook func()) {
	const (
		whCBT, hcbtCreateWnd                    = 5, 3
		wsPopup, wsClipChildren, wsClipSiblings = 0x80000000, 0x02000000, 0x04000000
		wsExToolWindow, wsExTopmost             = 0x00000080, 0x00000008
		gclpHbrBackground                       = ^uintptr(9) // -10
		graphite                                = 0x001a1414  // #14141a as COLORREF: the UI's dark background
	)
	cx, cy := CursorPos()
	x, y := placePopup(workAreaAt(cx, cy), popupW, popupH)
	brush, _, _ := pCreateSolidBrush.Call(graphite)

	proc := syscall.NewCallback(func(code int32, wParam, lParam uintptr) uintptr {
		if code == hcbtCreateWnd {
			buf := make([]uint16, 64)
			pGetClassName.Call(wParam, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if syscall.UTF16ToString(buf) == "webview" {
				// lParam is a pointer the OS hands to the hook (valid during this call).
				cs := (*cbtCreateWnd)(*(*unsafe.Pointer)(unsafe.Pointer(&lParam))).cs
				cs.x, cs.y = int32(x), int32(y)
				style := uint32(wsPopup | wsClipChildren | wsClipSiblings)
				cs.style = int32(style) // same bits; WS_POPUP is the sign bit
				cs.exStyle |= wsExToolWindow | wsExTopmost
				pSetClassLongPtr.Call(wParam, gclpHbrBackground, brush) // no white flash while WebView2 loads
			}
		}
		r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
		return r
	})
	tid, _, _ := pGetCurrentThreadID.Call()
	hook, _, _ := pSetWindowsHookEx.Call(whCBT, proc, 0, tid)
	return func() {
		if hook != 0 {
			pUnhookWindowsHookEx.Call(hook)
		}
	}
}
