//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW     = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pMoveWindow           = user32.NewProc("MoveWindow")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetWindowTextW       = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pSetFocus             = user32.NewProc("SetFocus")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pGetDC                = user32.NewProc("GetDC")
	pReleaseDC            = user32.NewProc("ReleaseDC")
	pSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	pCreateFontW    = gdi32.NewProc("CreateFontW")
	pGetDeviceCaps  = gdi32.NewProc("GetDeviceCaps")
	pSetBkColor     = gdi32.NewProc("SetBkColor")
	pGetStockObject = gdi32.NewProc("GetStockObject")

	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")

	pEnableWindow         = user32.NewProc("EnableWindow")
	pLoadLibraryW         = kernel32.NewProc("LoadLibraryW")
	pInitCommonControlsEx = syscall.NewLazyDLL("comctl32.dll").NewProc("InitCommonControlsEx")
	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pDragAcceptFiles      = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW       = shell32.NewProc("DragQueryFileW")
	pDragFinish           = shell32.NewProc("DragFinish")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsVScroll          = 0x00200000
	wsExClientEdge     = 0x00000200

	esMultiline   = 0x0004
	esAutoVScroll = 0x0040
	esAutoHScroll = 0x0080
	esReadOnly    = 0x0800
	esNumber      = 0x2000
	bsPushButton  = 0x0000
	bsDefPush     = 0x0001
	bsCheckbox    = 0x0003
	bmGetCheck    = 0x00F0
	bmSetCheck    = 0x00F1

	wmCreate         = 0x0001
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmSetFont        = 0x0030
	wmCommand        = 0x0111
	wmCtlColorStatic = 0x0138
	wmDropFiles      = 0x0233

	bnClicked = 0

	swShow = 5

	color3DFace   = 15
	idcArrow      = 32512
	logPixelsX    = 88
	whiteBrush    = 0
	ofnPathMust   = 0x00000800
	ofnFileMust   = 0x00001000
	ofnHideRO     = 0x00000004
	ofnExplorer   = 0x00080000
	ofnOverwrite  = 0x00000002
	cwUseDefault  = 0x80000000
	fontClearType = 5

	idEdit    = 100
	idBrowse  = 101
	idExtract = 102
	idResult  = 103
	idFlp     = 104
	idBar     = 105
	idLines   = 106
	idAll     = 107
)

type point struct{ x, y int32 }

type msgT struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type rect struct{ left, top, right, bottom int32 }

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

var (
	hMain, hLabel, hEdit, hBrowse, hExtract, hFlp, hBarLbl, hBar, hLinesLbl, hLines, hUp, hAll, hResult uintptr
	uiFont, monoFont                                                                                    uintptr
	dpi                                                                                                 = 96
)

func scale(v int) int { return v * dpi / 96 }

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return p
}

func makeFont(face string, pt int, weight int) uintptr {
	h := -(pt * dpi / 72)
	r, _, _ := pCreateFontW.Call(uintptr(int32(h)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, 0, 0, fontClearType, 0, uintptr(unsafe.Pointer(u16(face))))
	return r
}

func createChild(class, text string, style, exStyle uint32, id int) uintptr {
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16(class))),
		uintptr(unsafe.Pointer(u16(text))), uintptr(style|wsChild|wsVisible),
		0, 0, 10, 10, hMain, uintptr(id), 0, 0)
	return r
}

func setFont(h, f uintptr) { pSendMessageW.Call(h, wmSetFont, f, 1) }

func createControls(hwnd uintptr) {
	hMain = hwnd
	uiFont = makeFont("Segoe UI", 9, 400)
	monoFont = makeFont("Consolas", 10, 400)

	hLabel = createChild("STATIC", "Pro Tools session file (.ptx):", 0, 0, 0)
	hEdit = createChild("EDIT", "", wsTabStop|esAutoHScroll, wsExClientEdge, idEdit)
	hBrowse = createChild("BUTTON", "Browse...", wsTabStop|bsPushButton, 0, idBrowse)
	hExtract = createChild("BUTTON", "Extract tempo", wsTabStop|bsDefPush, 0, idExtract)
	hFlp = createChild("BUTTON", "Create FLP...", wsTabStop|bsPushButton, 0, idFlp)
	hBarLbl = createChild("STATIC", "Song starts at bar:", 0, 0, 0)
	hBar = createChild("EDIT", "1", wsTabStop|esAutoHScroll|esNumber, wsExClientEdge, idBar)
	hLinesLbl = createChild("STATIC", "Audio lines (0 = none):", 0, 0, 0)
	hLines = createChild("EDIT", "2", wsTabStop|esAutoHScroll|esNumber, wsExClientEdge, idLines)
	hAll = createChild("BUTTON", "All lines", wsTabStop|bsCheckbox, 0, idAll)
	initCommonControls()
	hUp = createChild("msctls_updown32", "", 0x0002|0x0004|0x0020|0x0080, 0, 0) // buddy int, align right, arrow keys, no thousands
	if hUp != 0 {
		pSendMessageW.Call(hUp, 0x046F, 0, 999) // UDM_SETRANGE32
		pSendMessageW.Call(hUp, 0x0469, hLines, 0)
		pSendMessageW.Call(hUp, 0x0471, 0, 2) // UDM_SETPOS32
	}
	hResult = createChild("EDIT", "", wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, wsExClientEdge, idResult)

	for _, h := range []uintptr{hLabel, hEdit, hBrowse, hExtract, hFlp, hBarLbl, hBar, hLinesLbl, hLines, hAll} {
		setFont(h, uiFont)
	}
	setFont(hResult, monoFont)
	pSendMessageW.Call(hResult, 0x00C5, 0, 0) // EM_LIMITTEXT: lift the 32K default so long tempo maps fit
	pDragAcceptFiles.Call(hwnd, 1)
}

func layout() {
	if hEdit == 0 || hResult == 0 {
		return
	}
	var rc rect
	pGetClientRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.right), int(rc.bottom)
	m := scale(12)
	rowH := scale(26)
	btnW := scale(96)
	y := m
	move := func(hw uintptr, x, y, cw, ch int) {
		pMoveWindow.Call(hw, uintptr(x), uintptr(y), uintptr(cw), uintptr(ch), 1)
	}
	move(hLabel, m, y, w-2*m, scale(18))
	y += scale(22)
	move(hEdit, m, y, w-3*m-btnW, rowH)
	move(hBrowse, w-m-btnW, y, btnW, rowH)
	y += rowH + scale(10)
	move(hExtract, m, y, scale(140), scale(30))
	move(hFlp, m+scale(140)+scale(8), y, scale(140), scale(30))
	bx := m + 2*scale(140) + scale(24)
	move(hBarLbl, bx, y+scale(6), scale(120), scale(20))
	move(hBar, bx+scale(124), y+scale(2), scale(60), scale(26))
	y += scale(30) + scale(10)
	move(hLinesLbl, m, y+scale(5), scale(150), scale(20))
	move(hLines, m+scale(154), y+scale(1), scale(60), scale(26))
	if hUp != 0 {
		pSendMessageW.Call(hUp, 0x0469, hLines, 0) // re-attach so the arrows follow the box
	}
	move(hAll, m+scale(154)+scale(60)+scale(24), y+scale(3), scale(110), scale(24))
	y += scale(26) + scale(12)
	rh := h - y - m
	if rh < scale(40) {
		rh = scale(40)
	}
	move(hResult, m, y, w-2*m, rh)
}

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	buf := make([]uint16, n+2)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s))))
}

func browse() {
	file := make([]uint16, 32768)
	filter := utf16.Encode([]rune("Pro Tools sessions (*.ptx)\x00*.ptx\x00All files (*.*)\x00*.*\x00\x00"))
	ofn := openFileName{
		hwndOwner:   hMain,
		lpstrFilter: &filter[0],
		lpstrFile:   &file[0],
		nMaxFile:    uint32(len(file)),
		lpstrTitle:  u16("Select a Pro Tools session"),
		flags:       ofnPathMust | ofnFileMust | ofnHideRO | ofnExplorer,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ok, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ok != 0 {
		setText(hEdit, syscall.UTF16ToString(file))
		extract()
	}
}

func runExtract(path string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("Unexpected error: %v", r)
		}
	}()
	path = strings.Trim(strings.TrimSpace(path), "\"")
	if path == "" {
		return "Choose a .ptx file first."
	}
	res, err := ExtractTempo(path)
	if err != nil {
		return "Could not read the file:\n" + err.Error()
	}
	return res.Format()
}

func extract() {
	setText(hResult, "Reading...")
	out := runExtract(getText(hEdit))
	if p := strings.Trim(strings.TrimSpace(getText(hEdit)), "\""); p != "" && !strings.HasPrefix(out, "Could not") {
		n, all := audioLineSetting()
		out += audioSummary(p, n, all)
	}
	setText(hResult, out)
}

type initCommonControlsExT struct{ size, icc uint32 }

func initCommonControls() {
	ic := initCommonControlsExT{icc: 0x00000010} // ICC_UPDOWN_CLASS
	ic.size = uint32(unsafe.Sizeof(ic))
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&ic)))
}

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// pickFolder shows the standard folder picker; it returns "" when cancelled.
func pickFolder(title string) string {
	name := make([]uint16, 520)
	bi := browseInfo{hwndOwner: hMain, pszDisplayName: &name[0], lpszTitle: u16(title), ulFlags: 0x1 | 0x40}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	buf := make([]uint16, 520)
	ok, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// audioLineSetting returns how many audio lines to copy: (n, all).
func audioLineSetting() (int, bool) {
	if c, _, _ := pSendMessageW.Call(hAll, bmGetCheck, 0, 0); c == 1 {
		return 0, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(getText(hLines)))
	if err != nil || n < 0 {
		n = 2
	}
	return n, false
}

func setLinesEnabled() {
	on := uintptr(1)
	if c, _, _ := pSendMessageW.Call(hAll, bmGetCheck, 0, 0); c == 1 {
		on = 0
	}
	pEnableWindow.Call(hLines, on)
	if hUp != 0 {
		pEnableWindow.Call(hUp, on)
	}
}

// audioSummary lists the audio lines that would be copied.
func audioSummary(path string, n int, all bool) string {
	if n == 0 && !all {
		return ""
	}
	au, err := ReadPTXAudio(path)
	if err != nil {
		return "\nAudio: " + err.Error() + "\n"
	}
	lines := au.Lines
	if !all && n < len(lines) {
		lines = lines[:n]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\nAudio lines with clips: %d found in the session, %d selected\n", len(au.Lines), len(lines))
	for i, l := range lines {
		first := float64(l.Clips[0].Start) / float64(au.SampleRate)
		fmt.Fprintf(&sb, "  line %d: %s  (%d clips, first at %s)\n", i+1, l.Name, len(l.Clips), formatClock(first))
	}
	return sb.String()
}

func createFLP() {
	path := strings.Trim(strings.TrimSpace(getText(hEdit)), "\"")
	if path == "" {
		setText(hResult, "Choose a .ptx file first.")
		return
	}
	res, err := ExtractTempo(path)
	if err != nil {
		setText(hResult, "Could not read the file:\n"+err.Error())
		return
	}
	text := res.Format()
	if len(res.Events) == 0 {
		setText(hResult, text+"\nNo FLP created.")
		return
	}

	def := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".flp"
	file := make([]uint16, 32768)
	copy(file, utf16.Encode([]rune(def)))
	filter := utf16.Encode([]rune("FL Studio project (*.flp)\x00*.flp\x00\x00"))
	ofn := openFileName{
		hwndOwner:       hMain,
		lpstrFilter:     &filter[0],
		lpstrFile:       &file[0],
		nMaxFile:        uint32(len(file)),
		lpstrInitialDir: u16(filepath.Dir(path)),
		lpstrTitle:      u16("Save FL Studio project"),
		lpstrDefExt:     u16("flp"),
		flags:           ofnPathMust | ofnHideRO | ofnExplorer | ofnOverwrite,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ok, _, _ := pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ok == 0 {
		setText(hResult, text)
		return
	}
	out := syscall.UTF16ToString(file)

	startBar := 1
	if n, err := strconv.Atoi(strings.TrimSpace(getText(hBar))); err == nil && n >= 1 {
		startBar = n
	}
	events := ShiftToStart(res.Events, startBar)
	distinct := false
	for _, e := range events {
		if e.BPM != events[0].BPM {
			distinct = true
		}
	}
	bpm, clamped := ClampFLTempo(events[0].BPM)

	// audio (optional)
	opts := BuildOptions{Events: res.Events, StartBar: startBar}
	audioNote := ""
	nLines, allLines := audioLineSetting()
	if nLines > 0 || allLines {
		dir := filepath.Join(filepath.Dir(path), "Audio Files")
		if st, e := os.Stat(dir); e != nil || !st.IsDir() {
			dir = pickFolder("Audio Files folder not found next to the .ptx. Select the session's Audio Files folder:")
		}
		if dir == "" {
			audioNote = "Audio not added: no Audio Files folder selected.\n"
		} else if au, e := ReadPTXAudio(path); e != nil {
			audioNote = "Audio not added: " + e.Error() + "\n"
		} else {
			opts.Audio, opts.AudioDir = au, dir
			if !allLines {
				opts.Lines = nLines
			}
		}
	}
	data, report, err := BuildFLPFull(opts)
	if err != nil && opts.Audio != nil { // fall back to a tempo-only project, but say so
		audioNote = "Audio not added: " + err.Error() + "\n"
		opts.Audio, report = nil, ""
		data, _, err = BuildFLPFull(opts)
	}
	if err != nil {
		setText(hResult, text+"\nCould not build the FLP:\n"+err.Error())
		return
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		setText(hResult, text+"\nCould not save the FLP:\n"+err.Error())
		return
	}
	text += "\nFL Studio project saved:\n" + out + "\n"
	if startBar > 1 {
		text += fmt.Sprintf("Song start set to bar %d: tempo events and clips were shifted %d bars earlier.\n", startBar, startBar-1)
	}
	text += "Project tempo set to " + formatBPM(bpm) + " BPM.\n"
	if clamped {
		text += "(Original tempo is outside FL Studio's 10-522 BPM range, so it was limited.)\n"
	}
	if distinct {
		where := "playlist track 1"
		if opts.Audio != nil {
			where = "the playlist line below the audio"
		}
		text += "Tempo automation clip added on " + where + " from bar 1:\n"
		for i, e := range events {
			if i == 0 || e.BPM != events[i-1].BPM {
				bar := int(e.Beats()/4) + 1
				text += fmt.Sprintf("  bar %d -> %s BPM\n", bar, formatBPM(e.BPM))
			}
		}
	}
	if opts.Audio != nil {
		text += "Audio clips copied to the playlist (same line order as Pro Tools):\n" + report
		if opts.Audio.OverlapsTrimmed > 0 {
			text += fmt.Sprintf("  %d overlapping clips in the session were trimmed so they do not play twice\n", opts.Audio.OverlapsTrimmed)
		}
	}
	text += audioNote
	setText(hResult, text)
}

func wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch uint32(msg) {
	case wmCreate:
		createControls(hwnd)
		return 0
	case wmSize:
		layout()
		return 0
	case wmCommand:
		if (wparam>>16)&0xffff == bnClicked {
			switch wparam & 0xffff {
			case idBrowse:
				browse()
			case idExtract, 1: // 1 = IDOK (Enter pressed in the path field)
				extract()
			case idFlp:
				createFLP()
			case idAll:
				setLinesEnabled()
			}
		}
		return 0
	case wmDropFiles:
		buf := make([]uint16, 32768)
		n, _, _ := pDragQueryFileW.Call(wparam, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		pDragFinish.Call(wparam)
		if n > 0 {
			setText(hEdit, syscall.UTF16ToString(buf))
			extract()
		}
		return 0
	case wmCtlColorStatic:
		if lparam == hResult && hResult != 0 { // keep the read-only results box white
			pSetBkColor.Call(wparam, 0xFFFFFF)
			b, _, _ := pGetStockObject.Call(whiteBrush)
			return b
		}
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func main() {
	runtime.LockOSThread()
	pSetProcessDPIAware.Call()
	if dc, _, _ := pGetDC.Call(0); dc != 0 {
		if v, _, _ := pGetDeviceCaps.Call(dc, logPixelsX); v > 0 {
			dpi = int(v)
		}
		pReleaseDC.Call(0, dc)
	}

	inst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	className := u16("PTXTempoExtractorWnd")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     inst,
		hCursor:       cursor,
		hbrBackground: color3DFace + 1,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if a, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); a == 0 {
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16("Could not register window class: "+err.Error()))),
			uintptr(unsafe.Pointer(u16("PTX Tempo Extractor"))), 0x10)
		os.Exit(1)
	}

	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16("PTX Tempo Extractor"))), wsOverlappedWindow,
		cwUseDefault, cwUseDefault, uintptr(scale(680)), uintptr(scale(520)), 0, 0, inst, 0)
	if hwnd == 0 {
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16("Could not create window: "+err.Error()))),
			uintptr(unsafe.Pointer(u16("PTX Tempo Extractor"))), 0x10)
		os.Exit(1)
	}
	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)

	// "Open with" / command-line path: fill the field and run straight away.
	if len(os.Args) > 1 {
		setText(hEdit, os.Args[1])
		extract()
	} else {
		pSetFocus.Call(hEdit)
	}

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if d, _, _ := pIsDialogMessageW.Call(hwnd, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
