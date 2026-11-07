package main

// Dark, flat look for the window: everything except the two text boxes is drawn here with
// GDI (double buffered), plus the hamburger options pop-out.

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	gdi          = syscall.NewLazyDLL("gdi32.dll")
	dwmapi       = syscall.NewLazyDLL("dwmapi.dll")
	uxtheme      = syscall.NewLazyDLL("uxtheme.dll")
	pBeginPaint  = user32.NewProc("BeginPaint")
	pEndPaint    = user32.NewProc("EndPaint")
	pFillRect    = user32.NewProc("FillRect")
	pInvalidate  = user32.NewProc("InvalidateRect")
	pSetCapture  = user32.NewProc("SetCapture")
	pRelCapture  = user32.NewProc("ReleaseCapture")
	pGetCursor   = user32.NewProc("GetCursorPos")
	pScreenToCl  = user32.NewProc("ScreenToClient")
	pClientToScr = user32.NewProc("ClientToScreen")
	pTrackMouse  = user32.NewProc("TrackMouseEvent")
	pSetCursor   = user32.NewProc("SetCursor")
	pGetFocus    = user32.NewProc("GetFocus")
	pSetWindowP  = user32.NewProc("SetWindowPos")
	pIsVisible   = user32.NewProc("IsWindowVisible")
	pPostMessage = user32.NewProc("PostMessageW")
	pDrawText    = user32.NewProc("DrawTextW")
	pGetTick     = kernel32.NewProc("GetTickCount")
	pCreatePen   = gdi.NewProc("CreatePen")
	pCreateBrush = gdi.NewProc("CreateSolidBrush")
	pDeleteObj   = gdi.NewProc("DeleteObject")
	pSelectObj   = gdi.NewProc("SelectObject")
	pRoundRect   = gdi.NewProc("RoundRect")
	pCompatDC    = gdi.NewProc("CreateCompatibleDC")
	pCompatBmp   = gdi.NewProc("CreateCompatibleBitmap")
	pBitBlt      = gdi.NewProc("BitBlt")
	pDeleteDC    = gdi.NewProc("DeleteDC")
	pSetBkMode   = gdi.NewProc("SetBkMode")
	pSetTextCol  = gdi.NewProc("SetTextColor")
	pShellExec   = shell32.NewProc("ShellExecuteW")
	pDwmAttr     = dwmapi.NewProc("DwmSetWindowAttribute")
	pSetTheme    = uxtheme.NewProc("SetWindowTheme")
)

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

var (
	colBg      = rgb(0x0F, 0x11, 0x15)
	colPanel   = rgb(0x17, 0x1A, 0x21)
	colBorder  = rgb(0x26, 0x2B, 0x36)
	colField   = rgb(0x10, 0x12, 0x18)
	colText    = rgb(0xE6, 0xE8, 0xEE)
	colMuted   = rgb(0x8B, 0x93, 0xA5)
	colAccent  = rgb(0x7B, 0x61, 0xFF)
	colAccentH = rgb(0x93, 0x7D, 0xFF)
	colAccentP = rgb(0x66, 0x4E, 0xE6)
	colBtn     = rgb(0x22, 0x27, 0x33)
	colBtnH    = rgb(0x2C, 0x33, 0x43)
	colBtnP    = rgb(0x1B, 0x1F, 0x29)
	colOff     = rgb(0x3A, 0x41, 0x52)
)

const (
	wmPaint        = 0x000F
	wmEraseBkgnd   = 0x0014
	wmSetCursor    = 0x0020
	wmActivate     = 0x0006
	wmMouseMove    = 0x0200
	wmLButtonDown  = 0x0201
	wmLButtonUp    = 0x0202
	wmMouseLeave   = 0x02A3
	wmCtlColorEdit = 0x0133
	wmAppRefresh   = 0x8001

	dtLeft       = 0x0000
	dtCenter     = 0x0001
	dtRight      = 0x0002
	dtVCenter    = 0x0004
	dtSingleLine = 0x0020
	dtEllipsis   = 0x8000
	dtNoPrefix   = 0x0800

	idMenu = 120

	btnSecondary = 0
	btnPrimary   = 1
	btnIcon      = 2
)

// cfg holds the options of the hamburger menu.
var cfg = struct {
	all, recoverTakes, fades, disk, openDone bool
	changed                                  bool
}{openDone: true}

type paintStruct struct {
	hdc      uintptr
	fErase   int32
	rcPaint  rect
	fRestore int32
	fIncUpd  int32
	reserved [32]byte
}

type trackMouse struct {
	size, flags uint32
	hwnd        uintptr
	hover       uint32
}

type btn struct {
	id    int
	rc    rect
	label string
	kind  int
}

var (
	fTitle, fSub, fBtn, fSmall, fRow uintptr
	brBg, brPanel, brField           uintptr
	btns                             []btn
	hover, pressed                   = -1, -1
	tracking                         bool
	hMenu, hMenuBar                  uintptr
	menuHover, menuPressed           = -1, -1
	menuHiddenAt                     uint32
	resultCard, sessionCard, fieldRc rect
	hintRc, subRc, verRc, titleRc    rect
)

func darkTitleBar(hwnd uintptr) {
	v := uint32(1)
	pDwmAttr.Call(hwnd, 20, uintptr(unsafe.Pointer(&v)), 4)
	pDwmAttr.Call(hwnd, 19, uintptr(unsafe.Pointer(&v)), 4)
}

func darkScroll(h uintptr) {
	pSetTheme.Call(h, uintptr(unsafe.Pointer(u16("DarkMode_Explorer"))), 0)
}

func openInExplorer(path string) {
	args := `/select,"` + path + `"`
	pShellExec.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16("explorer.exe"))),
		uintptr(unsafe.Pointer(u16(args))), 0, 1)
}

func childOf(parent uintptr, class, text string, style, exStyle uint32, id int) uintptr {
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16(class))),
		uintptr(unsafe.Pointer(u16(text))), uintptr(style|wsChild|wsVisible),
		0, 0, 10, 10, parent, uintptr(id), 0, 0)
	return r
}

func createControls(hwnd uintptr) {
	hMain = hwnd
	uiFont = makeFont("Segoe UI", 10, 400)
	monoFont = makeFont("Consolas", 10, 400)
	fTitle = makeFont("Segoe UI", 18, 700)
	fSub = makeFont("Segoe UI", 9, 400)
	fBtn = makeFont("Segoe UI", 10, 600)
	fSmall = makeFont("Segoe UI", 8, 700)
	fRow = makeFont("Segoe UI", 10, 400)
	brBg, _, _ = pCreateBrush.Call(colBg)
	brPanel, _, _ = pCreateBrush.Call(colPanel)
	brField, _, _ = pCreateBrush.Call(colField)

	hEdit = childOf(hwnd, "EDIT", "", wsTabStop|esAutoHScroll, 0, idEdit)
	hResult = childOf(hwnd, "EDIT", "", wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, 0, idResult)
	setFont(hEdit, uiFont)
	setFont(hResult, monoFont)
	darkScroll(hResult)
	pSendMessageW.Call(hResult, 0x00C5, 0, 0) // EM_LIMITTEXT: lift the 32K default so long tempo maps fit
	setText(hResult, "Select a .ptx file or a project folder to read its tempo.\n\nUse the menu (top right) for the audio and recovery options.")
	pDragAcceptFiles.Call(hwnd, 1)
	createMenu()
}

func layout() {
	if hEdit == 0 || hResult == 0 {
		return
	}
	var rc rect
	pGetClientRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.right), int(rc.bottom)
	m := scale(24)
	mv := func(hw uintptr, x, y, cw, ch int) {
		pMoveWindow.Call(hw, uintptr(x), uintptr(y), uintptr(cw), uintptr(ch), 1)
	}
	R := func(x, y, cw, ch int) rect { return rect{int32(x), int32(y), int32(x + cw), int32(y + ch)} }

	titleRc = R(m, scale(18), w-2*m-scale(120), scale(34))
	subRc = R(m, scale(54), w-2*m-scale(120), scale(20))
	menuBtn := R(w-m-scale(40), scale(20), scale(40), scale(40))
	verRc = R(w-m-scale(40)-scale(130), scale(30), scale(120), scale(20))

	y := scale(92)
	cardH := scale(150)
	sessionCard = R(m, y, w-2*m, cardH)
	fieldRc = R(m+scale(16), y+scale(38), w-2*m-scale(32), scale(40))
	// the single-line edit sits vertically centred in its field
	eh := scale(22)
	mv(hEdit, m+scale(30), y+scale(38)+(scale(40)-eh)/2, w-2*m-scale(60), eh)
	by := y + scale(92)
	b1 := R(m+scale(16), by, scale(160), scale(38))
	b2 := R(m+scale(16)+scale(160)+scale(10), by, scale(200), scale(38))
	hintRc = R(int(b2.right)+scale(14), by, w-m-scale(16)-int(b2.right)-scale(14), scale(38))
	y += cardH + scale(16)
	create := R(m, y, w-2*m, scale(50))
	y += scale(50) + scale(16)
	rh := h - y - m
	if rh < scale(80) {
		rh = scale(80)
	}
	resultCard = R(m, y, w-2*m, rh)
	mv(hResult, m+scale(14), y+scale(12), w-2*m-scale(28), rh-scale(24))

	btns = []btn{
		{idBrowse, b1, "Select .ptx file", btnSecondary},
		{idFolder, b2, "Select project folder", btnSecondary},
		{idFlp, create, "Create FLP", btnPrimary},
		{idMenu, menuBtn, "", btnIcon},
	}
	pInvalidate.Call(hMain, 0, 1)
}

// ---------------------------------------------------------------- drawing helpers

func fillRc(dc uintptr, rc rect, col uintptr) {
	br, _, _ := pCreateBrush.Call(col)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), br)
	pDeleteObj.Call(br)
}

func rrect(dc uintptr, rc rect, radius int, fill, border uintptr) {
	br, _, _ := pCreateBrush.Call(fill)
	pen, _, _ := pCreatePen.Call(0, 1, border)
	ob, _, _ := pSelectObj.Call(dc, br)
	op, _, _ := pSelectObj.Call(dc, pen)
	r := scale(radius)
	pRoundRect.Call(dc, uintptr(rc.left), uintptr(rc.top), uintptr(rc.right), uintptr(rc.bottom), uintptr(r), uintptr(r))
	pSelectObj.Call(dc, ob)
	pSelectObj.Call(dc, op)
	pDeleteObj.Call(br)
	pDeleteObj.Call(pen)
}

func drawText(dc uintptr, s string, rc rect, flags uint32, font, col uintptr) {
	of, _, _ := pSelectObj.Call(dc, font)
	pSetTextCol.Call(dc, col)
	pSetBkMode.Call(dc, 1) // transparent
	p := u16(s)
	pDrawText.Call(dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), uintptr(flags|dtNoPrefix))
	pSelectObj.Call(dc, of)
}

func drawButton(dc uintptr, b btn, hov, press bool) {
	switch b.kind {
	case btnIcon:
		fill, border := colPanel, colBorder
		if press {
			fill = colBtnP
		} else if hov {
			fill = colBtnH
		}
		rrect(dc, b.rc, 12, fill, border)
		cx := int(b.rc.left+b.rc.right) / 2
		cy := int(b.rc.top+b.rc.bottom) / 2
		lw, lh, gap := scale(18), scale(2), scale(5)
		for i := -1; i <= 1; i++ {
			y := cy + i*(lh+gap) - lh/2
			fillRc(dc, rect{int32(cx - lw/2), int32(y), int32(cx + lw/2), int32(y + lh)}, colText)
		}
	default:
		fill, border, txt := colBtn, colBorder, colText
		if b.kind == btnPrimary {
			fill, border, txt = colAccent, colAccent, rgb(0xFF, 0xFF, 0xFF)
			if press {
				fill, border = colAccentP, colAccentP
			} else if hov {
				fill, border = colAccentH, colAccentH
			}
		} else if press {
			fill = colBtnP
		} else if hov {
			fill = colBtnH
		}
		rrect(dc, b.rc, 10, fill, border)
		drawText(dc, b.label, b.rc, dtCenter|dtVCenter|dtSingleLine, fBtn, txt)
	}
}

// paintBuffered draws into an off-screen bitmap and copies it, so nothing flickers.
func paintBuffered(hwnd uintptr, draw func(dc uintptr, w, h int)) {
	var ps paintStruct
	dc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.right), int(rc.bottom)
	mem, _, _ := pCompatDC.Call(dc)
	bmp, _, _ := pCompatBmp.Call(dc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObj.Call(mem, bmp)
	draw(mem, w, h)
	pBitBlt.Call(dc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, 0x00CC0020) // SRCCOPY
	pSelectObj.Call(mem, old)
	pDeleteObj.Call(bmp)
	pDeleteDC.Call(mem)
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func paintMain(dc uintptr, w, h int) {
	fillRc(dc, rect{0, 0, int32(w), int32(h)}, colBg)
	drawText(dc, "PTX → FLP", titleRc, dtLeft|dtVCenter|dtSingleLine, fTitle, colText)
	drawText(dc, "Pro Tools sessions to FL Studio projects", subRc, dtLeft|dtVCenter|dtSingleLine, fSub, colMuted)
	drawText(dc, "v"+version, verRc, dtRight|dtVCenter|dtSingleLine, fSub, colMuted)

	rrect(dc, sessionCard, 16, colPanel, colBorder)
	lab := sessionCard
	lab.left += int32(scale(18))
	lab.top += int32(scale(12))
	lab.bottom = lab.top + int32(scale(18))
	drawText(dc, "SESSION", lab, dtLeft|dtVCenter|dtSingleLine, fSmall, colMuted)
	rrect(dc, fieldRc, 10, colField, colBorder)
	drawText(dc, "or drop a .ptx file / project folder on this window", hintRc, dtLeft|dtVCenter|dtSingleLine|dtEllipsis, fSub, colMuted)

	rrect(dc, resultCard, 16, colPanel, colBorder)
	for i, b := range btns {
		drawButton(dc, b, i == hover, i == pressed)
	}
}

func hitBtn(lparam uintptr) int {
	x, y := int32(int16(lparam&0xffff)), int32(int16((lparam>>16)&0xffff))
	for i, b := range btns {
		if x >= b.rc.left && x < b.rc.right && y >= b.rc.top && y < b.rc.bottom {
			return i
		}
	}
	return -1
}

func runBtn(id int) {
	switch id {
	case idBrowse:
		browse()
	case idFolder:
		browseFolder()
	case idFlp:
		createFLP()
	case idMenu:
		toggleMenu()
	}
}

// themeMainProc handles the painting, mouse and colour messages of the main window.
func themeMainProc(hwnd, msg, wparam, lparam uintptr) (uintptr, bool) {
	switch uint32(msg) {
	case wmEraseBkgnd:
		return 1, true
	case wmPaint:
		paintBuffered(hwnd, paintMain)
		return 0, true
	case wmMouseMove:
		if !tracking {
			tm := trackMouse{flags: 2, hwnd: hwnd}
			tm.size = uint32(unsafe.Sizeof(tm))
			pTrackMouse.Call(uintptr(unsafe.Pointer(&tm)))
			tracking = true
		}
		if i := hitBtn(lparam); i != hover {
			hover = i
			pInvalidate.Call(hwnd, 0, 0)
		}
		return 0, true
	case wmMouseLeave:
		tracking = false
		if hover != -1 {
			hover = -1
			pInvalidate.Call(hwnd, 0, 0)
		}
		return 0, true
	case wmSetCursor:
		if hover >= 0 {
			c, _, _ := pLoadCursorW.Call(0, 32649) // IDC_HAND
			pSetCursor.Call(c)
			return 1, true
		}
	case wmLButtonDown:
		if i := hitBtn(lparam); i >= 0 {
			pressed = i
			pSetCapture.Call(hwnd)
			pInvalidate.Call(hwnd, 0, 0)
		}
		return 0, true
	case wmLButtonUp:
		if pressed >= 0 {
			p := pressed
			pressed = -1
			pRelCapture.Call()
			pInvalidate.Call(hwnd, 0, 0)
			if hitBtn(lparam) == p {
				runBtn(btns[p].id)
			}
		}
		return 0, true
	case wmAppRefresh:
		if strings.TrimSpace(getText(hEdit)) != "" {
			extract()
		}
		return 0, true
	case wmCtlColorEdit, wmCtlColorStatic:
		return editColors(wparam, lparam), true
	}
	return 0, false
}

// editColors paints the two text boxes (and the menu's number boxes) dark.
func editColors(dc, ctl uintptr) uintptr {
	pSetTextCol.Call(dc, colText)
	pSetBkMode.Call(dc, 2) // opaque, so the text background matches
	bg, br := colField, brField
	if ctl == hResult {
		bg, br = colPanel, brPanel
	}
	if (ctl == hLines && cfg.all) || ctl == 0 {
		pSetTextCol.Call(dc, colMuted)
	}
	pSetBkCol(dc, bg)
	return br
}

func pSetBkCol(dc, col uintptr) { pSetBkColor.Call(dc, col) }

// ---------------------------------------------------------------- hamburger menu

type menuRow struct {
	label string
	flag  *bool    // switch rows
	edit  *uintptr // number rows
}

var menuRows []menuRow

const menuClass = "PTXToFLPMenu"

func createMenu() {
	inst, _, _ := pGetModuleHandleW.Call(0)
	cur, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(menuProc), hInstance: inst, hCursor: cur, lpszClassName: u16(menuClass)}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// WS_POPUP | WS_BORDER-less, tool window so it has no taskbar button
	hMenu, _, _ = pCreateWindowExW.Call(0x00000080, uintptr(unsafe.Pointer(u16(menuClass))), 0,
		0x80000000, 0, 0, uintptr(scale(360)), uintptr(scale(46+7*46+12)), hMain, 0, inst, 0)
	hBar = childOf(hMenu, "EDIT", "1", wsTabStop|esAutoHScroll|esNumber, 0, idBar)
	hLines = childOf(hMenu, "EDIT", "2", wsTabStop|esAutoHScroll|esNumber, 0, idLines)
	setFont(hBar, uiFont)
	setFont(hLines, uiFont)
	menuRows = []menuRow{
		{label: "Song starts at bar", edit: &hBar},
		{label: "Audio lines to copy (0 = none)", edit: &hLines},
		{label: "Copy all lines", flag: &cfg.all},
		{label: "Recover unplaced takes (best guess)", flag: &cfg.recoverTakes},
		{label: "Include fade pieces", flag: &cfg.fades},
		{label: "Add WAVs the .ptx does not know (folder)", flag: &cfg.disk},
		{label: "Open folder when done", flag: &cfg.openDone},
	}
	layoutMenu()
}

func menuRowRect(i, w int) rect {
	top := scale(46) + i*scale(46)
	return rect{0, int32(top), int32(w), int32(top + scale(46))}
}

func layoutMenu() {
	var rc rect
	pGetClientRect.Call(hMenu, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.right)
	for i, r := range menuRows {
		if r.edit == nil {
			continue
		}
		rr := menuRowRect(i, w)
		ew, eh := scale(60), scale(22)
		x := w - scale(16) - scale(8) - ew
		pMoveWindow.Call(*r.edit, uintptr(x), uintptr(int(rr.top)+(scale(46)-eh)/2), uintptr(ew), uintptr(eh), 1)
	}
}

func toggleMenu() {
	if v, _, _ := pIsVisible.Call(hMenu); v != 0 {
		pShowWindow.Call(hMenu, 0)
		return
	}
	// the click that closed the menu by taking focus must not reopen it straight away
	if t, _, _ := pGetTick.Call(); uint32(t)-menuHiddenAt < 250 {
		return
	}
	b := btns[len(btns)-1].rc
	pt := point{b.right, b.bottom + int32(scale(8))}
	pClientToScr.Call(hMain, uintptr(unsafe.Pointer(&pt)))
	var mr rect
	pGetClientRect.Call(hMenu, uintptr(unsafe.Pointer(&mr)))
	x := int(pt.x) - int(mr.right)
	cfg.changed = false
	pSetWindowP.Call(hMenu, 0, uintptr(x), uintptr(pt.y), 0, 0, 0x0001|0x0004|0x0040) // NOSIZE|NOZORDER|SHOWWINDOW
	pSetFocus.Call(hMenu)
}

func paintMenu(dc uintptr, w, h int) {
	fillRc(dc, rect{0, 0, int32(w), int32(h)}, colPanel)
	rrect(dc, rect{0, 0, int32(w), int32(h)}, 0, colPanel, colBorder)
	head := rect{int32(scale(18)), 0, int32(w), int32(scale(46))}
	drawText(dc, "OPTIONS", head, dtLeft|dtVCenter|dtSingleLine, fSmall, colMuted)
	for i, r := range menuRows {
		rr := menuRowRect(i, w)
		if i == menuHover && r.flag != nil {
			fillRc(dc, rr, colBtnH)
		}
		lab := rr
		lab.left += int32(scale(18))
		lab.right -= int32(scale(80))
		drawText(dc, r.label, lab, dtLeft|dtVCenter|dtSingleLine|dtEllipsis, fRow, colText)
		if r.flag != nil {
			sw, sh := scale(40), scale(22)
			x := int(rr.right) - scale(16) - sw
			y := int(rr.top) + (scale(46)-sh)/2
			sr := rect{int32(x), int32(y), int32(x + sw), int32(y + sh)}
			col := colOff
			if *r.flag {
				col = colAccent
			}
			rrect(dc, sr, 22, col, col)
			kd := sh - scale(6)
			kx := x + scale(3)
			if *r.flag {
				kx = x + sw - scale(3) - kd
			}
			rrect(dc, rect{int32(kx), int32(y + scale(3)), int32(kx + kd), int32(y + scale(3) + kd)}, 22, rgb(0xFF, 0xFF, 0xFF), rgb(0xFF, 0xFF, 0xFF))
		} else if r.edit != nil {
			ew, eh := scale(60), scale(22)
			x := w - scale(16) - scale(8) - ew
			y := int(rr.top) + (scale(46)-eh)/2
			rrect(dc, rect{int32(x - scale(6)), int32(y - scale(3)), int32(x + ew + scale(6)), int32(y + eh + scale(3))}, 8, colField, colBorder)
		}
	}
}

func menuRowAt(lparam uintptr, w int) int {
	y := int32(int16((lparam >> 16) & 0xffff))
	for i := range menuRows {
		rr := menuRowRect(i, w)
		if y >= rr.top && y < rr.bottom {
			return i
		}
	}
	return -1
}

func updateLinesEnabled() {
	on := uintptr(1)
	if cfg.all {
		on = 0
	}
	pEnableWindow.Call(hLines, on)
}

func menuProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.right)
	switch uint32(msg) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, paintMenu)
		return 0
	case wmActivate:
		if wparam&0xffff == 0 { // lost focus: close, and refresh the summary if an option changed
			pShowWindow.Call(hwnd, 0)
			t, _, _ := pGetTick.Call()
			menuHiddenAt = uint32(t)
			if cfg.changed {
				cfg.changed = false
				pPostMessage.Call(hMain, wmAppRefresh, 0, 0)
			}
		}
		return 0
	case wmMouseMove:
		if i := menuRowAt(lparam, w); i != menuHover {
			menuHover = i
			pInvalidate.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonUp:
		if i := menuRowAt(lparam, w); i >= 0 && menuRows[i].flag != nil {
			*menuRows[i].flag = !*menuRows[i].flag
			cfg.changed = true
			updateLinesEnabled()
			pInvalidate.Call(hwnd, 0, 0)
		}
		return 0
	case wmCtlColorEdit, wmCtlColorStatic:
		return editColors(wparam, lparam)
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}
