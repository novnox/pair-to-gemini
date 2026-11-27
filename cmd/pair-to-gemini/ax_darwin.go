//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>
#include <string.h>

static int ptg_ax_is_trusted(void) {
    return AXIsProcessTrusted() ? 1 : 0;
}

static int ptg_is_screen_locked(void) {
    CFDictionaryRef d = CGSessionCopyCurrentDictionary();
    if (!d) return 0;
    int locked = 0;
    CFStringRef k = CFStringCreateWithCString(NULL, "CGSSessionScreenIsLocked", kCFStringEncodingUTF8);
    if (k) {
        CFTypeRef val = CFDictionaryGetValue(d, k);
        if (val) {
            CFTypeID tid = CFGetTypeID(val);
            if (tid == CFBooleanGetTypeID()) {
                locked = CFBooleanGetValue((CFBooleanRef)val) ? 1 : 0;
            } else if (tid == CFNumberGetTypeID()) {
                int num = 0;
                if (CFNumberGetValue((CFNumberRef)val, kCFNumberIntType, &num)) {
                    locked = (num != 0) ? 1 : 0;
                }
            }
        }
        CFRelease(k);
    }
    CFRelease(d);
    return locked;
}

static AXUIElementRef ptg_app_el(int pid, float timeout_s) {
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (app) {
        AXUIElementSetMessagingTimeout(app, timeout_s);
    }
    return app;
}

static void ptg_cf_release(CFTypeRef ref) {
    if (ref) CFRelease(ref);
}

static CFTypeRef ptg_cf_retain(CFTypeRef ref) {
    if (ref) CFRetain(ref);
    return ref;
}

static char* ptg_cfstr_to_cstr(CFTypeRef ref) {
    if (!ref || CFGetTypeID(ref) != CFStringGetTypeID()) {
        return NULL;
    }
    CFStringRef s = (CFStringRef)ref;
    CFIndex len = CFStringGetLength(s);
    CFIndex cap = CFStringGetMaximumSizeForEncoding(len, kCFStringEncodingUTF8) + 1;
    if (cap < 64) cap = 64;
    char* buf = (char*)malloc((size_t)cap);
    if (!buf) return NULL;
    if (!CFStringGetCString(s, buf, cap, kCFStringEncodingUTF8)) {
        free(buf);
        return NULL;
    }
    return buf;
}

static char* ptg_ax_get_str(AXUIElementRef el, const char* attr) {
    if (!el || !attr) return NULL;
    CFStringRef k = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    if (!k) return NULL;
    CFTypeRef out = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, k, &out);
    CFRelease(k);
    if (err != kAXErrorSuccess || !out) return NULL;
    char* res = ptg_cfstr_to_cstr(out);
    CFRelease(out);
    return res;
}

static int ptg_ax_get_bool(AXUIElementRef el, const char* attr, int def_val) {
    if (!el || !attr) return def_val;
    CFStringRef k = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    if (!k) return def_val;
    CFTypeRef out = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, k, &out);
    CFRelease(k);
    if (err != kAXErrorSuccess || !out) return def_val;
    int res = def_val;
    if (CFGetTypeID(out) == CFBooleanGetTypeID()) {
        res = CFBooleanGetValue((CFBooleanRef)out) ? 1 : 0;
    }
    CFRelease(out);
    return res;
}

static int ptg_ax_set_str(AXUIElementRef el, const char* attr, const char* val) {
    if (!el || !attr || !val) return -1;
    CFStringRef k = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    if (!k) return -1;
    CFStringRef v = CFStringCreateWithCString(NULL, val, kCFStringEncodingUTF8);
    if (!v) {
        CFRelease(k);
        return -1;
    }
    AXError err = AXUIElementSetAttributeValue(el, k, v);
    CFRelease(v);
    CFRelease(k);
    return (int)err;
}

static int ptg_ax_set_bool(AXUIElementRef el, const char* attr, int val) {
    if (!el || !attr) return -1;
    CFStringRef k = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    if (!k) return -1;
    AXError err = AXUIElementSetAttributeValue(el, k, val ? kCFBooleanTrue : kCFBooleanFalse);
    CFRelease(k);
    return (int)err;
}

static CFTypeRef ptg_ax_copy_attr(AXUIElementRef el, const char* attr) {
    if (!el || !attr) return NULL;
    CFStringRef k = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    if (!k) return NULL;
    CFTypeRef out = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, k, &out);
    CFRelease(k);
    if (err != kAXErrorSuccess || !out) return NULL;
    return out;
}

static long ptg_cfarray_count(CFTypeRef ref) {
    if (!ref || CFGetTypeID(ref) != CFArrayGetTypeID()) return 0;
    return (long)CFArrayGetCount((CFArrayRef)ref);
}

static AXUIElementRef ptg_cfarray_get_el(CFTypeRef ref, long idx) {
    if (!ref || CFGetTypeID(ref) != CFArrayGetTypeID()) return NULL;
    return (AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)ref, (CFIndex)idx);
}

static int ptg_ax_perform(AXUIElementRef el, const char* action) {
    if (!el || !action) return -1;
    CFStringRef a = CFStringCreateWithCString(NULL, action, kCFStringEncodingUTF8);
    if (!a) return -1;
    AXError err = AXUIElementPerformAction(el, a);
    CFRelease(a);
    return (int)err;
}

static char* ptg_ax_actions_joined(AXUIElementRef el) {
    if (!el) return NULL;
    CFArrayRef arr = NULL;
    if (AXUIElementCopyActionNames(el, &arr) != kAXErrorSuccess || !arr) return NULL;
    CFIndex n = CFArrayGetCount(arr);
    size_t cap = 256;
    char* out = (char*)malloc(cap);
    if (!out) {
        CFRelease(arr);
        return NULL;
    }
    out[0] = '\0';
    size_t used = 0;
    for (CFIndex i = 0; i < n; i++) {
        CFTypeRef item = CFArrayGetValueAtIndex(arr, i);
        char* s = ptg_cfstr_to_cstr(item);
        if (s) {
            size_t sl = strlen(s);
            if (used + sl + 2 >= cap) {
                cap = (used + sl + 2) * 2;
                char* nb = (char*)realloc(out, cap);
                if (!nb) {
                    free(s);
                    break;
                }
                out = nb;
            }
            if (used > 0) out[used++] = '\n';
            memcpy(out + used, s, sl);
            used += sl;
            out[used] = '\0';
            free(s);
        }
    }
    CFRelease(arr);
    return out;
}
*/
import "C"

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

type AXElem struct {
	ref C.AXUIElementRef
}

func (e AXElem) Valid() bool { return e.ref != 0 }
func (e AXElem) Release() {
	if e.ref != 0 {
		C.ptg_cf_release(C.CFTypeRef(e.ref))
	}
}
func (e AXElem) Retain() AXElem {
	if e.ref != 0 {
		C.ptg_cf_retain(C.CFTypeRef(e.ref))
	}
	return e
}

func axIsTrusted() bool    { return C.ptg_ax_is_trusted() != 0 }
func isScreenLocked() bool { return C.ptg_is_screen_locked() != 0 }

func findGeminiPID() int {
	out, err := exec.Command("/usr/bin/pgrep", "-x", "Gemini").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if pid, err := strconv.Atoi(s); err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func appElem(pid int, timeoutSec float32) AXElem {
	return AXElem{ref: C.ptg_app_el(C.int(pid), C.float(timeoutSec))}
}

func axGetStr(el AXElem, attr string) string {
	if !el.Valid() {
		return ""
	}
	ca := C.CString(attr)
	defer C.free(unsafe.Pointer(ca))
	cs := C.ptg_ax_get_str(el.ref, ca)
	if cs == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
}

func axGetBool(el AXElem, attr string, defVal bool) bool {
	if !el.Valid() {
		return defVal
	}
	ca := C.CString(attr)
	defer C.free(unsafe.Pointer(ca))
	d := C.int(0)
	if defVal {
		d = 1
	}
	return C.ptg_ax_get_bool(el.ref, ca, d) != 0
}

func axSetStr(el AXElem, attr, val string) int {
	ca := C.CString(attr)
	cv := C.CString(val)
	defer C.free(unsafe.Pointer(ca))
	defer C.free(unsafe.Pointer(cv))
	return int(C.ptg_ax_set_str(el.ref, ca, cv))
}

func axSetBool(el AXElem, attr string, val bool) int {
	ca := C.CString(attr)
	defer C.free(unsafe.Pointer(ca))
	v := C.int(0)
	if val {
		v = 1
	}
	return int(C.ptg_ax_set_bool(el.ref, ca, v))
}

func axPerform(el AXElem, action string) int {
	ca := C.CString(action)
	defer C.free(unsafe.Pointer(ca))
	return int(C.ptg_ax_perform(el.ref, ca))
}

func axActions(el AXElem) []string {
	if !el.Valid() {
		return nil
	}
	cs := C.ptg_ax_actions_joined(el.ref)
	if cs == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cs))
	s := C.GoString(cs)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func hasAction(actions []string, target string) bool {
	for _, a := range actions {
		if a == target {
			return true
		}
	}
	return false
}

func withArrayAttr(el AXElem, attr string, fn func(items []AXElem)) bool {
	if !el.Valid() {
		return false
	}
	ca := C.CString(attr)
	defer C.free(unsafe.Pointer(ca))
	arr := C.ptg_ax_copy_attr(el.ref, ca)
	if arr == 0 {
		return false
	}
	defer C.ptg_cf_release(arr)
	n := int(C.ptg_cfarray_count(arr))
	if n == 0 {
		fn(nil)
		return true
	}
	if n > 300 {
		n = 300
	}
	items := make([]AXElem, 0, n)
	for i := 0; i < n; i++ {
		child := C.ptg_cfarray_get_el(arr, C.long(i))
		if child != 0 {
			items = append(items, AXElem{ref: child})
		}
	}
	fn(items)
	return true
}

func copyElemAttr(el AXElem, attr string) AXElem {
	if !el.Valid() {
		return AXElem{}
	}
	ca := C.CString(attr)
	defer C.free(unsafe.Pointer(ca))
	ref := C.ptg_ax_copy_attr(el.ref, ca)
	if ref == 0 {
		return AXElem{}
	}
	return AXElem{ref: C.AXUIElementRef(ref)}
}

func walkAX(root AXElem, skipOutline bool, maxDepth, maxNodes int, visit func(el AXElem, role string) bool) {
	visited := 0
	var rec func(el AXElem, depth int) bool
	rec = func(el AXElem, depth int) bool {
		if !el.Valid() || depth > maxDepth || visited >= maxNodes {
			return false
		}
		visited++
		role := axGetStr(el, "AXRole")
		if skipOutline && role == "AXOutline" {
			return false
		}
		if visit(el, role) {
			return true
		}
		stopped := false
		withArrayAttr(el, "AXChildren", func(children []AXElem) {
			limit := len(children)
			if limit > 200 {
				limit = 200
			}
			for i := 0; i < limit; i++ {
				if rec(children[i], depth+1) {
					stopped = true
					return
				}
			}
		})
		return stopped
	}
	rec(root, 0)
}

func findAX(root AXElem, skipOutline bool, maxDepth int, pred func(el AXElem, role string) bool) AXElem {
	var found AXElem
	walkAX(root, skipOutline, maxDepth, 1400, func(el AXElem, role string) bool {
		if pred(el, role) {
			found = el.Retain()
			return true
		}
		return false
	})
	return found
}

func isMinichatWindow(w AXElem) bool {
	sub := axGetStr(w, "AXSubrole")
	if sub == "AXSystemDialog" {
		return true
	}
	id := strings.ToLower(axGetStr(w, "AXIdentifier"))
	title := strings.ToLower(axGetStr(w, "AXTitle"))
	return strings.Contains(id, "minichat") || strings.Contains(title, "mini chat") || strings.Contains(title, "迷你对话")
}

func findMainWindow(app AXElem) AXElem {
	var best AXElem
	var p2, p3, p4 AXElem
	withArrayAttr(app, "AXWindows", func(wins []AXElem) {
		for _, w := range wins {
			if best.Valid() {
				return
			}
			if axGetStr(w, "AXRole") != "AXWindow" || isMinichatWindow(w) {
				continue
			}
			sub := axGetStr(w, "AXSubrole")
			title := strings.TrimSpace(axGetStr(w, "AXTitle"))
			if sub == "AXStandardWindow" && strings.HasPrefix(title, "Gemini") {
				best = w.Retain()
				return
			}
			if sub == "AXStandardWindow" && title != "" && !p2.Valid() {
				p2 = w
			} else if sub != "AXDialog" && title != "" && !p3.Valid() {
				p3 = w
			} else if sub == "AXStandardWindow" && !p4.Valid() {
				p4 = w
			}
		}
		if !best.Valid() {
			if p2.Valid() {
				best = p2.Retain()
			} else if p3.Valid() {
				best = p3.Retain()
			} else if p4.Valid() {
				best = p4.Retain()
			}
		}
	})
	return best
}

func getWindowTitle(pid int) string {
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return ""
	}
	defer win.Release()
	return NormalizeWindowTitle(axGetStr(win, "AXTitle"))
}

func ensureMainWindowSilent(pid int) bool {
	app := appElem(pid, 1.5)
	defer app.Release()
	keepHidden := axGetBool(app, "AXHidden", false)
	if w := findMainWindow(app); w.Valid() {
		w.Release()
		if keepHidden {
			axSetBool(app, "AXHidden", true)
		}
		return true
	}
	mb := copyElemAttr(app, "AXMenuBar")
	if !mb.Valid() {
		return false
	}
	defer mb.Release()
	openItem := findAX(mb, false, 5, func(el AXElem, role string) bool {
		if role != "AXMenuItem" {
			return false
		}
		t := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXTitle")))
		return t == "打开应用" || t == "open app"
	})
	if !openItem.Valid() {
		return false
	}
	rc := axPerform(openItem, "AXPress")
	openItem.Release()
	if rc != 0 {
		return false
	}
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if keepHidden {
			axSetBool(app, "AXHidden", true)
		}
		if w := findMainWindow(app); w.Valid() {
			w.Release()
			if keepHidden {
				axSetBool(app, "AXHidden", true)
			}
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func ensureSidebarOpen(win AXElem) AXElem {
	outline := findAX(win, false, 10, func(_ AXElem, role string) bool { return role == "AXOutline" })
	if outline.Valid() {
		return outline
	}
	toggle := findAX(win, false, 8, func(el AXElem, role string) bool {
		return role == "AXButton" && axGetStr(el, "AXIdentifier") == "sidebar_toggle"
	})
	if toggle.Valid() {
		axPerform(toggle, "AXPress")
		toggle.Release()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			outline = findAX(win, false, 10, func(_ AXElem, role string) bool { return role == "AXOutline" })
			if outline.Valid() {
				return outline
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return AXElem{}
}

type retainedRowBtn struct {
	btn  AXElem
	item SidebarItem
}

func iterSidebarButtons(outline AXElem, maxItems int, stopTitle string) []retainedRowBtn {
	var out []retainedRowBtn
	seenFilter := false
	processRows := func(rows []AXElem) {
		capRows := len(rows)
		limit := maxItems + 25
		if limit < 60 {
			limit = 60
		}
		if capRows > limit {
			capRows = limit
		}
		for i := 0; i < capRows; i++ {
			row := rows[i]
			if !seenFilter {
				f := findAX(row, false, 4, func(el AXElem, _ string) bool {
					return axGetStr(el, "AXIdentifier") == "sidebar.taskFilterMenu"
				})
				if f.Valid() {
					f.Release()
					seenFilter = true
					continue
				}
			}
			btn := findAX(row, false, 4, func(_ AXElem, r string) bool { return r == "AXButton" })
			if !btn.Valid() {
				continue
			}
			raw := strings.TrimSpace(axGetStr(btn, "AXDescription"))
			if raw == "" {
				raw = strings.TrimSpace(axGetStr(btn, "AXTitle"))
			}
			if raw == "" {
				btn.Release()
				continue
			}
			parsed := ParseSidebarRowLabel(raw)
			if !seenFilter || sidebarStaticLabels[parsed.Title] {
				if !hasAction(axActions(btn), "AXShowMenu") {
					btn.Release()
					continue
				}
			}
			parsed.Source = "sidebar"
			if stopTitle != "" {
				if parsed.Title == stopTitle {
					out = append(out, retainedRowBtn{btn: btn, item: parsed})
					return
				}
				btn.Release()
				continue
			}
			out = append(out, retainedRowBtn{btn: btn, item: parsed})
			if len(out) >= maxItems {
				return
			}
		}
	}
	if !withArrayAttr(outline, "AXRows", processRows) {
		withArrayAttr(outline, "AXChildren", processRows)
	}
	return out
}

// busyConversationCount counts chats that are generating right now: sidebar
// badges plus the active window when it is busy. The active row often has no
// badge, so it is counted separately.
func busyConversationCount(pid int) int {
	active := NormalizeWindowTitle(getWindowTitle(pid))
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return 0
	}
	defer win.Release()
	count := 0
	activeCounted := false
	if outline := ensureSidebarOpen(win); outline.Valid() {
		for _, b := range iterSidebarButtons(outline, 40, "") {
			if b.item.Badge == "in_progress" {
				count++
				if b.item.Title == active {
					activeCounted = true
				}
			}
			b.btn.Release()
		}
		outline.Release()
	}
	if snap := pollConversation(pid); snap.Busy && !activeCounted {
		count++
	}
	return count
}

func getConversationBadge(pid int, targetTitle string) string {
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return ""
	}
	defer win.Release()
	outline := findAX(win, false, 10, func(_ AXElem, r string) bool { return r == "AXOutline" })
	if !outline.Valid() {
		return ""
	}
	defer outline.Release()
	btns := iterSidebarButtons(outline, 40, targetTitle)
	badge := ""
	for _, b := range btns {
		if b.item.Title == targetTitle {
			badge = b.item.Badge
		}
		b.btn.Release()
	}
	return badge
}

func listConversations(pid int) (hasWin bool, curTitle string, items []SidebarItem) {
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	seen := map[string]bool{}
	if win.Valid() {
		hasWin = true
		curTitle = NormalizeWindowTitle(axGetStr(win, "AXTitle"))
		outline := findAX(win, false, 10, func(_ AXElem, r string) bool { return r == "AXOutline" })
		if outline.Valid() {
			btns := iterSidebarButtons(outline, 40, "")
			for _, b := range btns {
				if b.item.Title != "" && !seen[b.item.Title] {
					seen[b.item.Title] = true
					items = append(items, b.item)
				}
				b.btn.Release()
			}
			outline.Release()
		}
		win.Release()
	}
	if len(items) == 0 {
		mb := copyElemAttr(app, "AXMenuBar")
		if mb.Valid() {
			skip := map[string]bool{"近期对话": true, "查看所有对话": true, "搜索": true, "recent chats": true, "view all chats": true, "search": true, "recent": true}
			chatMenu := findAX(mb, false, 3, func(el AXElem, r string) bool {
				if r != "AXMenuBarItem" {
					return false
				}
				t := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXTitle")))
				return t == "对话" || t == "chats" || t == "conversations"
			})
			if chatMenu.Valid() {
				walkAX(chatMenu, false, 4, 100, func(el AXElem, r string) bool {
					if r == "AXMenuItem" {
						t := strings.TrimSpace(axGetStr(el, "AXTitle"))
						if t != "" && !skip[strings.ToLower(t)] && !seen[t] {
							seen[t] = true
							items = append(items, SidebarItem{Raw: t, Title: t, Source: "menubar"})
						}
					}
					return false
				})
				chatMenu.Release()
			}
			mb.Release()
		}
	}
	return hasWin, curTitle, items
}

func searchAndOpenConversation(app, win AXElem, target string) bool {
	findSearchField := func() AXElem {
		var found AXElem
		withArrayAttr(app, "AXWindows", func(wins []AXElem) {
			for _, w := range wins {
				if found.Valid() {
					return
				}
				f := findAX(w, false, 10, func(el AXElem, r string) bool {
					return (r == "AXTextField" || r == "AXSearchField") && axGetStr(el, "AXIdentifier") == "gemini_search_text_field"
				})
				if f.Valid() {
					found = f
				}
			}
		})
		if found.Valid() {
			return found
		}
		return findAX(win, false, 12, func(el AXElem, r string) bool {
			return (r == "AXTextField" || r == "AXSearchField") && axGetStr(el, "AXIdentifier") != "settings_search_field"
		})
	}

	field := findSearchField()
	if !field.Valid() {
		sb := findAX(win, false, 10, func(el AXElem, r string) bool {
			return r == "AXButton" && axGetStr(el, "AXIdentifier") == "sidebar_header_search"
		})
		if !sb.Valid() {
			return false
		}
		rc := axPerform(sb, "AXPress")
		sb.Release()
		if rc != 0 {
			return false
		}
		deadline := time.Now().Add(1200 * time.Millisecond)
		for time.Now().Before(deadline) {
			field = findSearchField()
			if field.Valid() {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !field.Valid() {
		return false
	}
	defer field.Release()

	axSetBool(field, "AXFocused", true)
	axSetStr(field, "AXValue", target)

	isMatch := func(el AXElem, role string) bool {
		if role != "AXButton" && role != "AXRow" && role != "AXStaticText" && role != "AXMenuItem" {
			return false
		}
		id := axGetStr(el, "AXIdentifier")
		desc := strings.TrimSpace(axGetStr(el, "AXDescription"))
		if strings.HasPrefix(id, "gemini_search_row_") && (desc == target || strings.HasPrefix(desc, target+"、") || strings.HasPrefix(desc, target+",")) {
			return true
		}
		for _, attr := range []string{"AXTitle", "AXDescription", "AXValue"} {
			v := strings.TrimSpace(axGetStr(el, attr))
			if v != "" && ParseSidebarRowLabel(v).Title == target {
				return true
			}
		}
		return false
	}

	deadline := time.Now().Add(1800 * time.Millisecond)
	for time.Now().Before(deadline) {
		var hit AXElem
		withArrayAttr(app, "AXWindows", func(wins []AXElem) {
			for _, w := range wins {
				if hit.Valid() {
					return
				}
				hit = findAX(w, false, 14, isMatch)
			}
		})
		if !hit.Valid() {
			hit = findAX(win, false, 14, isMatch)
		}
		if hit.Valid() {
			rc := axPerform(hit, "AXPress")
			hit.Release()
			if rc == 0 {
				return true
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	axPerform(field, "AXCancel")
	return false
}

func openConversation(pid int, targetTitle string) (bool, string) {
	target := strings.TrimSpace(targetTitle)
	if target == "" {
		return false, "empty_target_title"
	}
	if !ensureMainWindowSilent(pid) {
		return false, "no_main_window"
	}
	if getWindowTitle(pid) == target {
		return true, "already_open"
	}
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return false, "no_main_window"
	}
	defer win.Release()

	// Gemini can switch chats yet report the press as unsupported (-25205),
	// so judge each attempt by the window title, not the return code.
	pressed := false
	if outline := ensureSidebarOpen(win); outline.Valid() {
		for _, b := range iterSidebarButtons(outline, 40, target) {
			if b.item.Title == target && !pressed {
				axPerform(b.btn, "AXPress")
				pressed = true
			}
			b.btn.Release()
		}
		outline.Release()
	}
	if pressed && waitWindowTitle(pid, target, 3*time.Second) {
		return true, "sidebar"
	}
	searched := searchAndOpenConversation(app, win, target)
	if searched && waitWindowTitle(pid, target, 3*time.Second) {
		return true, "search"
	}
	if !pressed && !searched {
		return false, "conversation_not_found"
	}
	return false, "title_verification_failed"
}

func waitWindowTitle(pid int, target string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if getWindowTitle(pid) == target {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return false
}

func findComposer(win AXElem) AXElem {
	ta := findAX(win, true, 12, func(el AXElem, r string) bool {
		if r != "AXTextArea" {
			return false
		}
		d := strings.ToLower(axGetStr(el, "AXDescription"))
		for _, tok := range []string{"问问 gemini", "分配任务", "描述任务", "接下来做什么", "ask gemini", "assign a task", "describe a task", "message gemini", "what’s next", "what's next"} {
			if strings.Contains(d, tok) {
				return true
			}
		}
		return false
	})
	if ta.Valid() {
		return ta
	}
	return findAX(win, true, 12, func(_ AXElem, r string) bool { return r == "AXTextArea" })
}

func isSendButton(el AXElem, role string) bool {
	if role != "AXButton" {
		return false
	}
	id := strings.TrimSpace(axGetStr(el, "AXIdentifier"))
	switch id {
	case "mic_button", "stc_button", "sidebar_toggle", "sidebar_header_search", "tempChat_button", "collapse_button", "tab_stop_model_selector":
		return false
	case "send_button":
		return axGetBool(el, "AXEnabled", true)
	}
	d := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXDescription")))
	t := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXTitle")))
	for _, m := range []string{"提交", "发送", "submit", "send", "send message"} {
		if d == m || t == m {
			return axGetBool(el, "AXEnabled", true)
		}
	}
	return false
}

func isStopElement(el AXElem, role string) bool {
	if role != "AXButton" {
		return false
	}
	id := strings.TrimSpace(axGetStr(el, "AXIdentifier"))
	if id == "stop_button" || id == "AGENT_WORK_PANE_STOP_BUTTON" {
		return true
	}
	d := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXDescription")))
	t := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXTitle")))
	lbl := t + " " + d
	return d == "停止" || d == "停止回答" || d == "stop" || d == "stop response" || d == "stop generating" ||
		t == "停止" || t == "停止回答" || t == "stop" || t == "stop response" || t == "stop generating" ||
		strings.Contains(lbl, "停止回答") || strings.Contains(lbl, "stop response") || strings.Contains(lbl, "stop generating")
}

func writePromptAndSubmit(pid int, prompt string) (bool, string) {
	if strings.TrimSpace(prompt) == "" {
		return false, "empty_prompt"
	}
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return false, "no_main_window"
	}
	defer win.Release()

	ta := findComposer(win)
	if !ta.Valid() {
		return false, "composer_not_found"
	}
	axSetBool(ta, "AXFocused", true)
	axSetStr(ta, "AXValue", "")
	errRc := axSetStr(ta, "AXValue", prompt)
	actual := axGetStr(ta, "AXValue")
	ta.Release()
	if errRc != 0 {
		return false, "ax_set_value_failed"
	}
	prefix := prompt
	if len(prefix) > 32 {
		prefix = prefix[:32]
	}
	if actual == "" || !strings.HasPrefix(actual, prefix) {
		return false, "ax_value_verify_failed"
	}

	sent := false
	attempted := false
	btnDeadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(btnDeadline) {
		btn := findAX(win, true, 14, isSendButton)
		if btn.Valid() {
			attempted = true
			rc := axPerform(btn, "AXPress")
			btn.Release()
			if rc == 0 {
				sent = true
				break
			}
			return false, "submit_press_uncertain"
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sent {
		if attempted {
			return false, "submit_press_uncertain"
		}
		return false, "send_button_not_found_or_disabled"
	}

	verDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(verDeadline) {
		if st := findAX(win, true, 14, isStopElement); st.Valid() {
			st.Release()
			return true, "submitted_stop_visible"
		}
		ta2 := findComposer(win)
		if ta2.Valid() {
			cv := strings.TrimSpace(axGetStr(ta2, "AXValue"))
			ta2.Release()
			switch cv {
			case "", "问问 Gemini", "分配任务", "描述任务", "接下来做什么？", "Ask Gemini", "Assign a task", "Describe a task", "Message Gemini", "What’s next?", "What's next?":
				return true, "submitted_composer_cleared"
			}
		}
		time.Sleep(60 * time.Millisecond)
	}
	return false, "submit_unconfirmed"
}

type PollSnapshot struct {
	HasWindow        bool
	WindowTitle      string
	Busy             bool
	ConfirmationCard string
	Modal            bool
	HasCopyButton    bool
	Chunks           []string
}

func pollConversation(pid int) PollSnapshot {
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return PollSnapshot{}
	}
	defer win.Release()

	snap := PollSnapshot{
		HasWindow:   true,
		WindowTitle: NormalizeWindowTitle(axGetStr(win, "AXTitle")),
	}
	// A sheet is a modal dialog (rename, settings, ...). Its text must never be
	// mistaken for a reply, and input would go nowhere useful.
	withArrayAttr(win, "AXSheets", func(items []AXElem) {
		if len(items) > 0 {
			snap.Modal = true
		}
	})
	walkAX(win, true, 24, 1400, func(el AXElem, role string) bool {
		switch role {
		case "AXTextArea", "AXTextField", "AXSearchField", "AXMenuBar", "AXToolbar":
			return false
		}
		id := axGetStr(el, "AXIdentifier")
		desc := axGetStr(el, "AXDescription")
		title := axGetStr(el, "AXTitle")
		if id != "" {
			if id == "stop_button" || id == "AGENT_WORK_PANE_STOP_BUTTON" {
				snap.Busy = true
			}
			for _, cid := range confirmationCardIDs {
				if strings.Contains(id, cid) {
					snap.ConfirmationCard = cid
					break
				}
			}
		}
		if isStopElement(el, role) {
			snap.Busy = true
			return false
		}
		extract := false
		switch role {
		case "AXButton":
			dl := strings.ToLower(strings.TrimSpace(desc))
			tl := strings.ToLower(strings.TrimSpace(title))
			if id == "copy-button" || dl == "拷贝" || dl == "copy" || dl == "copy response" || tl == "拷贝" || tl == "copy" || tl == "copy response" {
				snap.HasCopyButton = true
			} else if strings.Contains(id, "copy-button") {
				// A finished reply is a button whose description holds the
				// whole answer as Markdown. Its identifier varies by version
				// ("copy-button-copy-button", "code-widget-…-copy-button-…").
				extract = true
			}
		case "AXHeading", "AXStaticText", "AXWebArea":
			extract = true
		case "AXGroup":
			v := axGetStr(el, "AXValue")
			if strings.Contains(v, "Generating your response") || strings.Contains(v, "正在生成回答") {
				snap.Busy = true
			}
		}
		if extract {
			axVal := axGetStr(el, "AXValue")
			val := axVal
			if strings.Contains(desc, "[[TASK_") || (desc != "" && len(desc) > len(axVal)) {
				val = desc
			} else if val == "" {
				if desc != "" {
					val = desc
				} else {
					val = title
				}
			}
			vs := strings.TrimSpace(val)
			if vs != "" && !chromeTexts[vs] &&
				!strings.HasSuffix(vs, "，想聊点什么？") &&
				!strings.HasPrefix(vs, "Hi ") &&
				!strings.Contains(vs, "what’s on your mind?") &&
				!strings.Contains(vs, "what's on your mind?") {
				if strings.Contains(vs, "Generating your response") || strings.Contains(vs, "正在生成回答") {
					snap.Busy = true
				}
				if len(snap.Chunks) == 0 || snap.Chunks[len(snap.Chunks)-1] != vs {
					snap.Chunks = append(snap.Chunks, vs)
				}
			}
		}
		return false
	})
	return snap
}

func pressNewChat(pid int) bool {
	app := appElem(pid, 1.5)
	defer app.Release()
	triggered := false
	if win := findMainWindow(app); win.Valid() {
		btn := findAX(win, false, 10, func(el AXElem, r string) bool {
			if r != "AXButton" {
				return false
			}
			d := strings.TrimSpace(axGetStr(el, "AXDescription"))
			t := strings.TrimSpace(axGetStr(el, "AXTitle"))
			id := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXIdentifier")))
			return d == "发起新对话" || d == "New chat" || d == "New Chat" ||
				t == "发起新对话" || t == "New chat" || t == "New Chat" ||
				id == "new-chat" || id == "new_chat" || id == "actions_bar_new_chat"
		})
		if btn.Valid() {
			if !axGetBool(btn, "AXEnabled", true) {
				btn.Release()
				win.Release()
				return true
			}
			triggered = axPerform(btn, "AXPress") == 0
			btn.Release()
		}
		win.Release()
	}
	if !triggered {
		mb := copyElemAttr(app, "AXMenuBar")
		if mb.Valid() {
			item := findAX(mb, false, 4, func(el AXElem, r string) bool {
				if r != "AXMenuItem" {
					return false
				}
				t := strings.TrimSpace(axGetStr(el, "AXTitle"))
				return t == "发起新对话" || t == "New chat" || t == "New Chat"
			})
			if item.Valid() {
				if !axGetBool(item, "AXEnabled", true) {
					item.Release()
					mb.Release()
					return true
				}
				triggered = axPerform(item, "AXPress") == 0
				item.Release()
			}
			mb.Release()
		}
	}
	if !triggered {
		return false
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if newChatTitles[getWindowTitle(pid)] {
			return true
		}
		time.Sleep(60 * time.Millisecond)
	}
	return newChatTitles[getWindowTitle(pid)]
}

func ensureTaskMode(pid int) bool {
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return false
	}
	defer win.Release()

	isTaskActive := func() bool {
		b := findAX(win, true, 12, func(el AXElem, r string) bool {
			return r == "AXButton" && axGetStr(el, "AXIdentifier") == "chat-input-mode-assign"
		})
		if b.Valid() {
			sel := axGetBool(b, "AXSelected", false)
			b.Release()
			if sel {
				return true
			}
		}
		ta := findComposer(win)
		if ta.Valid() {
			d := strings.ToLower(axGetStr(ta, "AXDescription"))
			ta.Release()
			for _, tok := range []string{"分配任务", "描述任务", "assign a task", "describe a task"} {
				if strings.Contains(d, tok) {
					return true
				}
			}
		}
		return false
	}

	if isTaskActive() {
		return true
	}
	btn := findAX(win, true, 12, func(el AXElem, r string) bool {
		if r != "AXButton" {
			return false
		}
		d := strings.TrimSpace(axGetStr(el, "AXDescription"))
		return axGetStr(el, "AXIdentifier") == "chat-input-mode-assign" || d == "任务" || d == "Task"
	})
	if !btn.Valid() {
		return false
	}
	rc := axPerform(btn, "AXPress")
	btn.Release()
	if rc != 0 {
		return false
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if isTaskActive() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return isTaskActive()
}

func attachWorkspaceFolder(pid int, workspacePath string) (bool, string) {
	absDir, err := filepath.Abs(workspacePath)
	if err != nil {
		return false, "invalid_workspace_path"
	}
	targetName := filepath.Base(absDir)
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return false, "no_main_window"
	}
	defer win.Release()

	hasChip := func() bool {
		matched := false
		walkAX(win, true, 12, 1400, func(el AXElem, r string) bool {
			if r == "AXButton" || r == "AXGroup" {
				for _, attr := range []string{"AXValue", "AXTitle", "AXDescription"} {
					c := strings.TrimSpace(axGetStr(el, attr))
					if c == absDir || c == targetName || c == "Remove "+targetName || c == "移除 "+targetName || c == "Connected folder: "+targetName {
						matched = true
						return true
					}
				}
			}
			return false
		})
		return matched
	}
	if hasChip() {
		return true, "already_attached"
	}

	findFoldersBtn := func() AXElem {
		return findAX(win, true, 12, func(el AXElem, r string) bool {
			if r != "AXButton" {
				return false
			}
			d := strings.TrimSpace(axGetStr(el, "AXDescription"))
			t := strings.TrimSpace(axGetStr(el, "AXTitle"))
			return d == "文件夹" || d == "Folders" || t == "文件夹" || t == "Folders"
		})
	}

	fb := findFoldersBtn()
	if !fb.Valid() {
		rm := findAX(win, true, 12, func(el AXElem, r string) bool {
			if r != "AXButton" {
				return false
			}
			d := strings.TrimSpace(axGetStr(el, "AXDescription"))
			t := strings.TrimSpace(axGetStr(el, "AXTitle"))
			return d == "移除" || d == "Remove" || t == "移除" || t == "Remove"
		})
		if rm.Valid() {
			axPerform(rm, "AXPress")
			rm.Release()
			deadline := time.Now().Add(1500 * time.Millisecond)
			for time.Now().Before(deadline) {
				fb = findFoldersBtn()
				if fb.Valid() {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	if !fb.Valid() {
		return false, "folders_button_not_found"
	}
	defer fb.Release()

	fbBg := fb.Retain()
	go func() {
		defer fbBg.Release()
		axPerform(fbBg, "AXPress")
	}()

	var menu AXElem
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		menu = findAX(fb, false, 4, func(_ AXElem, r string) bool { return r == "AXMenu" })
		if !menu.Valid() {
			menu = findAX(win, true, 10, func(_ AXElem, r string) bool { return r == "AXMenu" })
		}
		if menu.Valid() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !menu.Valid() {
		return false, "folders_menu_did_not_open"
	}
	defer menu.Release()

	var matched AXElem
	var nameMatches []AXElem
	walkAX(menu, false, 8, 200, func(el AXElem, r string) bool {
		if r != "AXMenuItem" {
			return false
		}
		t := strings.TrimSpace(axGetStr(el, "AXTitle"))
		d := strings.TrimSpace(axGetStr(el, "AXDescription"))
		for _, cand := range []string{t, d} {
			if cand == absDir {
				matched = el.Retain()
				return true
			}
			if cand == targetName {
				nameMatches = append(nameMatches, el.Retain())
			}
		}
		return false
	})
	defer func() {
		for _, nm := range nameMatches {
			nm.Release()
		}
	}()
	if !matched.Valid() && len(nameMatches) == 1 {
		matched = nameMatches[0].Retain()
	}
	if !matched.Valid() {
		axPerform(menu, "AXCancel")
		return false, "workspace_not_bookmarked_in_gemini"
	}
	defer matched.Release()
	if axPerform(matched, "AXPress") != 0 {
		return false, "folder_item_press_failed"
	}
	chipDeadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(chipDeadline) {
		if hasChip() {
			return true, "attached"
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true, "attached_unverified"
}

func renameConversation(pid int, currentTitle, newTitle string) (bool, string) {
	if currentTitle == newTitle {
		return true, currentTitle
	}
	app := appElem(pid, 1.5)
	defer app.Release()
	win := findMainWindow(app)
	if !win.Valid() {
		return false, currentTitle
	}
	defer win.Release()

	outline := ensureSidebarOpen(win)
	if !outline.Valid() {
		return false, currentTitle
	}
	btns := iterSidebarButtons(outline, 40, currentTitle)
	outline.Release()
	var targetBtn AXElem
	for _, b := range btns {
		if b.item.Title == currentTitle && !targetBtn.Valid() {
			targetBtn = b.btn
		} else {
			b.btn.Release()
		}
	}
	if !targetBtn.Valid() {
		return false, currentTitle
	}
	defer targetBtn.Release()

	bgBtn := targetBtn.Retain()
	go func() {
		defer bgBtn.Release()
		axPerform(bgBtn, "AXShowMenu")
	}()

	var menu, renameItem AXElem
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		menu = findAX(targetBtn, false, 4, func(_ AXElem, r string) bool { return r == "AXMenu" })
		if !menu.Valid() {
			menu = findAX(win, false, 10, func(_ AXElem, r string) bool { return r == "AXMenu" })
		}
		if menu.Valid() {
			renameItem = findAX(menu, false, 4, func(el AXElem, r string) bool {
				if r != "AXMenuItem" {
					return false
				}
				t := strings.ToLower(strings.TrimSpace(axGetStr(el, "AXTitle")))
				return t == "重命名" || t == "rename" || t == "重命名…" || t == "rename…"
			})
			if renameItem.Valid() {
				break
			}
			menu.Release()
			menu = AXElem{}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !renameItem.Valid() {
		if menu.Valid() {
			axPerform(menu, "AXCancel")
			menu.Release()
		}
		return false, currentTitle
	}
	rc := axPerform(renameItem, "AXPress")
	renameItem.Release()
	menu.Release()
	if rc != 0 {
		return false, currentTitle
	}

	renamed := false
	dlgDeadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(dlgDeadline) && !renamed {
		var containers []AXElem
		withArrayAttr(win, "AXSheets", func(sheets []AXElem) {
			for _, s := range sheets {
				containers = append(containers, s.Retain())
			}
		})
		withArrayAttr(app, "AXWindows", func(wins []AXElem) {
			for _, w := range wins {
				t := strings.TrimSpace(axGetStr(w, "AXTitle"))
				if t == "重命名此对话" || t == "重命名此任务" || t == "Rename this chat" || t == "Rename this task" {
					containers = append(containers, w.Retain())
				}
			}
		})
		if len(containers) == 0 {
			containers = append(containers, win.Retain())
		}
		for _, c := range containers {
			if !renamed {
				tf := findAX(c, false, 14, func(el AXElem, r string) bool {
					id := axGetStr(el, "AXIdentifier")
					return r == "AXTextField" && id != "settings_search_field" && id != "gemini_search_text_field"
				})
				if tf.Valid() {
					axSetBool(tf, "AXFocused", true)
					axSetStr(tf, "AXValue", newTitle)
					tf.Release()
					okBtn := findAX(c, false, 14, func(el AXElem, r string) bool {
						if r != "AXButton" {
							return false
						}
						id := strings.TrimSpace(axGetStr(el, "AXIdentifier"))
						t := strings.TrimSpace(axGetStr(el, "AXTitle"))
						d := strings.TrimSpace(axGetStr(el, "AXDescription"))
						return id == "action-button-1" || t == "确认" || t == "确定" || t == "保存" || t == "Confirm" || t == "OK" || t == "Save" ||
							d == "确认" || d == "确定" || d == "保存" || d == "Confirm" || d == "OK" || d == "Save"
					})
					if okBtn.Valid() {
						axPerform(okBtn, "AXPress")
						okBtn.Release()
						renamed = true
					}
				}
			}
			c.Release()
		}
		if !renamed {
			time.Sleep(80 * time.Millisecond)
		}
	}

	if renamed {
		waitEnd := time.Now().Add(12 * time.Second)
		for time.Now().Before(waitEnd) {
			if getWindowTitle(pid) == newTitle {
				return true, newTitle
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	actual := getWindowTitle(pid)
	if actual == "" {
		actual = currentTitle
	}
	return actual == newTitle, actual
}

func plausibleTitle(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" || newChatTitles[t] {
		return false
	}
	if strings.HasPrefix(t, "[task ") {
		return false
	}
	return len([]rune(t)) <= 120
}

// captureStableTitle waits for the window header to move past the provisional
// prompt title to the name Gemini generates (observed within a few seconds).
// Callers hold the UI lock, so no other task can switch the window meanwhile.
func captureStableTitle(pid int) string {
	deadline := time.Now().Add(10 * time.Second)
	for {
		t := NormalizeWindowTitle(getWindowTitle(pid))
		if plausibleTitle(t) && !strings.HasPrefix(t, "[task ") && !strings.HasPrefix(t, "[[TASK_") {
			return t
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func startNewConversation(pid int) (bool, string) {
	if !ensureMainWindowSilent(pid) {
		return false, "no_main_window"
	}
	if !pressNewChat(pid) {
		return false, "new_chat_failed"
	}
	if !ensureTaskMode(pid) {
		return false, "switch_task_mode_failed"
	}
	return true, ""
}

// createTaskConversation opens a new Task chat, sends initPrompt, waits for
// Gemini to name it, and renames it to targetTitle when possible. It returns
// the final title and how it was set, or a non-empty failReason.
func createTaskConversation(pid int, targetTitle, initPrompt, workspace string, waitSec float64) (title, mode, failReason string) {
	if ok, reason := startNewConversation(pid); !ok {
		return "", "", reason
	}
	if workspace != "" {
		if ok, reason := attachWorkspaceFolder(pid, workspace); !ok {
			return "", "", reason
		}
	}
	if ok, reason := writePromptAndSubmit(pid, initPrompt); !ok {
		return "", "", reason
	}
	if waitSec < 10 {
		waitSec = 10
	}
	deadline := time.Now().Add(time.Duration(waitSec * float64(time.Second)))
	var settledSince time.Time
	actualTitle := ""
	for time.Now().Before(deadline) {
		snap := pollConversation(pid)
		if snap.ConfirmationCard != "" {
			return "", "", "init_requires_confirmation"
		}
		actualTitle = snap.WindowTitle
		if !snap.Busy && snap.HasCopyButton && !newChatTitles[actualTitle] {
			if settledSince.IsZero() {
				settledSince = time.Now()
			} else if time.Since(settledSince) >= time.Second {
				break
			}
		} else {
			settledSince = time.Time{}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if newChatTitles[actualTitle] {
		actualTitle = getWindowTitle(pid)
	}
	if newChatTitles[actualTitle] {
		return "", "", "auto_title_not_generated"
	}
	okRen, finalTitle := renameConversation(pid, actualTitle, targetTitle)
	if okRen {
		return finalTitle, "renamed", ""
	}
	return finalTitle, "auto_titled", ""
}

func geminiAppVersion() string {
	for _, p := range []string{
		"/Applications/Gemini.app/Contents/Info.plist",
		filepath.Join(os.Getenv("HOME"), "Applications/Gemini.app/Contents/Info.plist"),
	} {
		if _, err := os.Stat(p); err == nil {
			out, err := exec.Command("/usr/bin/defaults", "read", p, "CFBundleShortVersionString").Output()
			if err == nil {
				return strings.TrimSpace(string(out))
			}
		}
	}
	return ""
}
