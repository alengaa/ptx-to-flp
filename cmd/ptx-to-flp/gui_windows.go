//go:build windows

package main

import (
	"github.com/alengaa/ptx-to-flp/internal/flp"
	"github.com/alengaa/ptx-to-flp/internal/ptx"

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
	ole32    = syscall.NewLazyDLL("ole32.dll")

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
	pOleInitialize        = ole32.NewProc("OleInitialize")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pCoCreateInstance     = ole32.NewProc("CoCreateInstance")
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
	idRecover = 108
	idFolder  = 109
	idFades   = 110
	idDisk    = 111
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
	hMain, hEdit, hBar, hLines, hResult uintptr
	uiFont, monoFont                    uintptr
	dpi                                 = 96
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

func browseFolder() {
	if p := pickFolder("Select the Pro Tools project folder (the one that contains the .ptx):"); p != "" {
		delete(chosenPTX, p) // a new pick asks again if the folder holds several .ptx
		setText(hEdit, p)
		extract()
	}
}

func isFolder(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func checked(h uintptr) bool {
	c, _, _ := pSendMessageW.Call(h, bmGetCheck, 0, 0)
	return c == 1
}

// source is the resolved input: a .ptx file, or a project folder and the .ptx found in it.
type source struct {
	ptxPath string
	folder  string // project folder, "" in .ptx-file mode
	note    string
}

func resolveSource(p string) (source, error) {
	p = strings.Trim(strings.TrimSpace(p), "\"")
	if p == "" {
		return source{}, fmt.Errorf("Choose a .ptx file or a project folder first.")
	}
	ptxPath, n, err := ptx.ResolveSessionInput(p)
	if err != nil {
		return source{}, err
	}
	picked := false
	if c, ok := chosenPTX[p]; ok && isFolder(p) {
		if _, e := os.Stat(filepath.Join(p, c)); e == nil {
			ptxPath, picked = filepath.Join(p, c), true
		}
	}
	s := source{ptxPath: ptxPath}
	if isFolder(p) {
		s.folder = p
		s.note = "Project folder mode: using " + filepath.Base(ptxPath)
		if picked {
			s.note += fmt.Sprintf(" (your choice of %d .ptx files)", n)
		} else if n > 1 {
			s.note += fmt.Sprintf(" (the most recently saved of %d .ptx files)", n)
		}
		s.note += "\n\n"
	} else {
		s.note = ".ptx file mode (limited: only what the .ptx itself contains)\n\n"
	}
	return s, nil
}

func runExtract(src source) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("Unexpected error: %v", r)
		}
	}()
	res, err := ptx.ExtractTempo(src.ptxPath)
	if err != nil {
		return "Could not read the file:\n" + err.Error()
	}
	return src.note + res.Format()
}

func extract() {
	ensureChoice(getText(hEdit))
	setText(hResult, "Reading...")
	src, err := resolveSource(getText(hEdit))
	if err != nil {
		setText(hResult, "Could not read the file:\n"+err.Error())
		return
	}
	out := runExtract(src)
	if !strings.HasPrefix(out, "Could not") && !strings.HasPrefix(out, "Unexpected") {
		out += audioSummary(src, readUI())
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
	if p, ok := pickFolderModern(title); ok {
		return p
	}
	return pickFolderLegacy(title)
}

type guid struct {
	d1 uint32
	d2 uint16
	d3 uint16
	d4 [8]byte
}

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog   = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

// vcall calls method number idx of the COM object obj.
func vcall(obj uintptr, idx int, args ...uintptr) uintptr {
	vt := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

// pickFolderModern shows the same Explorer-style dialog as the .ptx picker, in folder mode.
// ok is false when the dialog could not be created (the caller then uses the legacy tree).
func pickFolderModern(title string) (path string, ok bool) {
	pOleInitialize.Call(0)
	var dlg uintptr
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if int32(hr) < 0 || dlg == 0 {
		return "", false
	}
	defer vcall(dlg, 2) // Release
	var opts uint32
	vcall(dlg, 10, uintptr(unsafe.Pointer(&opts)))      // GetOptions
	vcall(dlg, 9, uintptr(opts|0x20|0x40))              // SetOptions: FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM
	vcall(dlg, 17, uintptr(unsafe.Pointer(u16(title)))) // SetTitle
	if r := vcall(dlg, 3, hMain); int32(r) < 0 {        // Show
		return "", int32(r) == -2147023673 // 0x800704C7: cancelled, still handled
	}
	var item uintptr
	if int32(vcall(dlg, 20, uintptr(unsafe.Pointer(&item)))) < 0 || item == 0 { // GetResult
		return "", true
	}
	defer vcall(item, 2)
	var pw uintptr
	if int32(vcall(item, 5, 0x80058000, uintptr(unsafe.Pointer(&pw)))) < 0 || pw == 0 { // GetDisplayName(FILESYSPATH)
		return "", true
	}
	defer pCoTaskMemFree.Call(pw)
	var out []uint16
	for p := pw; ; p += 2 {
		c := *(*uint16)(unsafe.Pointer(p))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return syscall.UTF16ToString(out), true
}

// pickFolderLegacy is the fallback when the folder dialog cannot be created: the regular
// file-open window. Open the project folder (the .ptx files show) and click Open.
func pickFolderLegacy(title string) string {
	const ofnNoValidate = 0x100
	file := utf16.Encode([]rune("Select this folder"))
	buf := make([]uint16, 32768)
	copy(buf, file)
	filter := utf16.Encode([]rune("Pro Tools sessions (*.ptx)\x00*.ptx\x00\x00"))
	ofn := openFileName{
		hwndOwner:   hMain,
		lpstrFilter: &filter[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  u16("Open the project folder (the one with the .ptx), then click Open"),
		flags:       ofnPathMust | ofnHideRO | ofnExplorer | ofnNoValidate,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if ok, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		return ""
	}
	p := syscall.UTF16ToString(buf)
	if !isFolder(p) {
		p = filepath.Dir(p)
	}
	return p
}

// chosenPTX remembers which .ptx the user picked for a project folder that holds several.
var chosenPTX = map[string]string{}

// choosePTX asks which .ptx to use when the folder holds more than one.
func choosePTX(folder string, files []string) string {
	buf := make([]uint16, 32768)
	filter := utf16.Encode([]rune("Pro Tools sessions\x00" + strings.Join(files, ";") + "\x00\x00"))
	ofn := openFileName{
		hwndOwner:       hMain,
		lpstrFilter:     &filter[0],
		lpstrFile:       &buf[0],
		nMaxFile:        uint32(len(buf)),
		lpstrInitialDir: u16(folder),
		lpstrTitle:      u16("Which PTX project would you like to use?"),
		flags:           ofnPathMust | ofnFileMust | ofnHideRO | ofnExplorer,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if ok, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		return ""
	}
	got := syscall.UTF16ToString(buf)
	if !strings.EqualFold(filepath.Dir(got), filepath.Clean(folder)) {
		return ""
	}
	return filepath.Base(got)
}

// ensureChoice asks once per folder which .ptx to use when there are several.
func ensureChoice(p string) {
	p = strings.Trim(strings.TrimSpace(p), "\"")
	if !isFolder(p) {
		return
	}
	files, err := ptx.ListSessionFiles(p)
	if err != nil || len(files) < 2 {
		return
	}
	if c, ok := chosenPTX[p]; ok {
		for _, f := range files {
			if f == c {
				return
			}
		}
	}
	if c := choosePTX(p, files); c != "" {
		chosenPTX[p] = c
	}
}

// uiOpts are the audio settings currently shown in the window.
type uiOpts struct {
	lines   int // lines to copy when !all
	all     bool
	recover bool // regions in the clip list that are on no playlist
	fades   bool
	disk    bool // WAVs on disk the .ptx does not know (project folder mode)
}

func readUI() uiOpts {
	u := uiOpts{recover: cfg.recoverTakes, fades: cfg.fades, disk: cfg.disk}
	if cfg.all {
		u.all = true
		return u
	}
	n, err := strconv.Atoi(strings.TrimSpace(getText(hLines)))
	if err != nil || n < 0 {
		n = 2
	}
	u.lines = n
	return u
}

// audioSummary lists what the audio options would copy.
func audioSummary(src source, u uiOpts) string {
	diskOn := u.disk && src.folder != ""
	if u.lines == 0 && !u.all && !u.recover && !diskOn && src.folder == "" {
		return ""
	}
	au, err := ptx.ReadPTXAudioOpts(src.ptxPath, ptx.ReadOptions{IncludeFades: u.fades})
	if err != nil && (au == nil || src.folder == "") {
		return "\nAudio: " + err.Error() + "\n"
	}
	var sb strings.Builder
	lines := au.Lines
	if !u.all && u.lines < len(lines) {
		lines = lines[:u.lines]
	}
	if u.lines == 0 && !u.all {
		lines = nil
	}
	if len(lines) > 0 || (len(au.Unplaced) == 0 && !diskOn) {
		fmt.Fprintf(&sb, "\nAudio lines with clips: %d found in the session, %d selected\n", len(au.Lines), len(lines))
	} else {
		sb.WriteString("\n")
	}
	for i, l := range lines {
		first := float64(l.Clips[0].Start) / float64(au.SampleRate)
		fmt.Fprintf(&sb, "  line %d: %s  (%d clips, first at %s)\n", i+1, l.Name, len(l.Clips), ptx.FormatClock(first))
	}
	if len(au.Unplaced) > 0 {
		fmt.Fprintf(&sb, "Takes in the clip list but on no timeline: %d, would use %d extra lines if recovered\n",
			len(au.Unplaced), len(ptx.PackLanes(au.Unplaced)))
	}
	if src.folder != "" {
		dir, how := ptx.FindSessionAudioDir(src.folder)
		if dir == "" {
			sb.WriteString("Audio Files folder not found in the project folder: you will be asked for it when creating the FLP.\n")
		} else if sc, e := ptx.ScanAudioDir(dir, au); e == nil {
			fmt.Fprintf(&sb, "Audio found in the %s: %d WAV files\n", how, sc.OnDisk)
			if n := len(sc.NotRegistered); n > 0 {
				fmt.Fprintf(&sb, "WARNING: %d WAV files there are not in this .ptx (it may be an older save).\n", n)
				sb.WriteString("  Tick the last checkbox to place them from their recording time.\n")
			}
		}
	}
	return sb.String()
}

func createFLP() {
	src, err := resolveSource(getText(hEdit))
	if err != nil {
		setText(hResult, err.Error())
		return
	}
	path := src.ptxPath
	res, err := ptx.ExtractTempo(path)
	if err != nil {
		setText(hResult, "Could not read the file:\n"+err.Error())
		return
	}
	text := src.note + res.Format()
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
	events := flp.ShiftToStart(res.Events, startBar)
	distinct := false
	for _, e := range events {
		if e.BPM != events[0].BPM {
			distinct = true
		}
	}
	bpm, clamped := flp.ClampFLTempo(events[0].BPM)

	// audio (optional)
	opts := flp.BuildOptions{Events: res.Events, StartBar: startBar}
	audioNote := ""
	u := readUI()
	diskOn := u.disk && src.folder != ""
	if u.lines > 0 || u.all || u.recover || diskOn {
		var dir string
		if src.folder != "" {
			var how string
			dir, how = ptx.FindSessionAudioDir(src.folder)
			if dir == "" {
				dir = pickFolder("Audio Files not found. Select the session's Audio Files folder:")
			} else {
				audioNote += "Audio read from the " + how + ".\n"
			}
		} else {
			dir = filepath.Join(filepath.Dir(path), "Audio Files")
			if st, e := os.Stat(dir); e != nil || !st.IsDir() {
				dir = pickFolder("Audio Files folder not found next to the .ptx. Select the session's Audio Files folder:")
			}
		}
		au, e := ptx.ReadPTXAudioOpts(path, ptx.ReadOptions{IncludeFades: u.fades})
		switch {
		case dir == "":
			audioNote += "Audio not added: no Audio Files folder selected.\n"
		case e != nil && (au == nil || !diskOn):
			audioNote += "Audio not added: " + e.Error() + "\n"
		default:
			opts.Audio, opts.AudioDir = au, dir
			opts.RecoverTakes = u.recover
			if !u.all {
				opts.Lines = u.lines
				if u.lines == 0 {
					opts.Lines = -1 // no regular lines, extra takes only
				}
			}
			if diskOn {
				added, skipped, e := ptx.AddDiskTakes(au, dir)
				if e != nil {
					audioNote += "WAVs not in the .ptx could not be scanned: " + e.Error() + "\n"
				} else {
					opts.AddDiskTakes = true
					audioNote += fmt.Sprintf("%d WAV files not in the .ptx were added from their recording time.\n", added)
					if len(skipped) > 0 {
						audioNote += fmt.Sprintf("%d WAV files could not be placed (skipped):\n", len(skipped))
						for i, s := range skipped {
							if i == 20 {
								audioNote += fmt.Sprintf("  ... and %d more\n", len(skipped)-20)
								break
							}
							audioNote += "  " + s.Name + ": " + s.Reason + "\n"
						}
					}
				}
			}
		}
	}
	data, report, err := flp.BuildFLPFull(opts)
	if err != nil && opts.Audio != nil { // fall back to a tempo-only project, but say so
		audioNote += "Audio not added: " + err.Error() + "\n"
		opts.Audio, report = nil, ""
		data, _, err = flp.BuildFLPFull(opts)
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
	text += "Project tempo set to " + ptx.FormatBPM(bpm) + " BPM.\n"
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
				text += fmt.Sprintf("  bar %d -> %s BPM\n", bar, ptx.FormatBPM(e.BPM))
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
	if cfg.openDone {
		openInExplorer(out)
	}
}

func wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch uint32(msg) {
	case wmCreate:
		createControls(hwnd)
		return 0
	case wmSize:
		layout()
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
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	if r, handled := themeMainProc(hwnd, msg, wparam, lparam); handled {
		return r
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
	className := u16("PTXToFLPWnd")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     inst,
		hCursor:       cursor,
		hbrBackground: 0,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if a, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); a == 0 {
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16("Could not register window class: "+err.Error()))),
			uintptr(unsafe.Pointer(u16(appTitle()))), 0x10)
		os.Exit(1)
	}

	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16(appTitle()))), wsOverlappedWindow,
		cwUseDefault, cwUseDefault, uintptr(scale(780)), uintptr(scale(720)), 0, 0, inst, 0)
	if hwnd == 0 {
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16("Could not create window: "+err.Error()))),
			uintptr(unsafe.Pointer(u16(appTitle()))), 0x10)
		os.Exit(1)
	}
	darkTitleBar(hwnd)
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
		if m.message == 0x0100 && m.wParam == 0x0D { // Enter in the path field reads the session
			if f, _, _ := pGetFocus.Call(); f == hEdit {
				extract()
				continue
			}
		}
		if d, _, _ := pIsDialogMessageW.Call(hwnd, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
