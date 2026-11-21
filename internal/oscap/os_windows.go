package oscap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows: the Win32 API for the system, disks and processes, and
// PowerShell (always there) for the recycle bin, the clipboard,
// notifications and screenshots.

func isHidden(_ string, fi fs.FileInfo) bool {
	if strings.HasPrefix(fi.Name(), ".") {
		return true
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
	}
	return false
}

func userDirs(home string) map[string]string {
	out := map[string]string{}
	for k, id := range map[string]*windows.KNOWNFOLDERID{
		"desktop": windows.FOLDERID_Desktop, "documents": windows.FOLDERID_Documents, "downloads": windows.FOLDERID_Downloads,
		"pictures": windows.FOLDERID_Pictures, "music": windows.FOLDERID_Music, "videos": windows.FOLDERID_Videos,
		"applications": windows.FOLDERID_ProgramFiles, "appdata": windows.FOLDERID_RoamingAppData, "localappdata": windows.FOLDERID_LocalAppData,
	} {
		if p, err := windows.KnownFolderPath(id, 0); err == nil {
			out[k] = p
		}
	}
	if out["downloads"] == "" {
		out["downloads"] = filepath.Join(home, "Downloads")
	}
	return out
}

func fillInfo(i *SystemInfo) {
	v := windows.RtlGetVersion()
	i.Version = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	i.Name = "Windows"
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		if s, _, err := k.GetStringValue("ProductName"); err == nil {
			i.Name = s
		}
		if s, _, err := k.GetStringValue("DisplayVersion"); err == nil && s != "" {
			i.Version += " (" + s + ")"
		}
		k.Close()
	}
	// (Windows 11 still says "Windows 10" in ProductName; its builds start at 22000)
	if v.BuildNumber >= 22000 && strings.HasPrefix(i.Name, "Windows 10") {
		i.Name = "Windows 11" + strings.TrimPrefix(i.Name, "Windows 10")
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		if s, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			i.CPUModel = strings.TrimSpace(s)
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		if s, _, err := k.GetStringValue("SystemProductName"); err == nil {
			i.Model = strings.TrimSpace(s)
		}
		k.Close()
	}
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		i.Memory = m.TotalPhys
	}
	if r, _, _ := procGetTickCount64.Call(); r != 0 {
		i.Uptime = int64(r / 1000)
	}
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	psapi                    = windows.NewLazySystemDLL("psapi.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
	procGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func disks() ([]Disk, error) {
	buf := make([]uint16, 512)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil {
		return nil, err
	}
	var out []Disk
	for _, root := range strings.Split(windows.UTF16ToString(nil)+string(utf16Runes(buf[:n])), "\x00") {
		if root == "" {
			continue
		}
		r, _ := windows.UTF16PtrFromString(root)
		kind := windows.GetDriveType(r)
		if kind == windows.DRIVE_NO_ROOT_DIR || kind == windows.DRIVE_CDROM && !ready(r) {
			continue
		}
		var avail, total, free uint64
		if windows.GetDiskFreeSpaceEx(r, &avail, &total, &free) != nil {
			continue
		}
		label, fsName := make([]uint16, 256), make([]uint16, 64)
		var serial, maxLen, flags uint32
		_ = windows.GetVolumeInformation(r, &label[0], uint32(len(label)), &serial, &maxLen, &flags, &fsName[0], uint32(len(fsName)))
		out = append(out, Disk{
			Mount: root, Device: strings.TrimSuffix(root, `\`), FS: strings.ToLower(windows.UTF16ToString(fsName)), Label: windows.UTF16ToString(label),
			Total: total, Free: free, Available: avail,
			Removable: kind == windows.DRIVE_REMOVABLE, Network: kind == windows.DRIVE_REMOTE, ReadOnly: flags&windows.FILE_READ_ONLY_VOLUME != 0,
		})
	}
	return out, nil
}

func ready(root *uint16) bool {
	var a, t, f uint64
	return windows.GetDiskFreeSpaceEx(root, &a, &t, &f) == nil
}

func utf16Runes(b []uint16) []rune {
	out := make([]rune, len(b))
	for i, c := range b {
		out[i] = rune(c)
	}
	return out
}

func isVolumeRoot(p string) bool {
	return len(p) <= 3 && len(p) >= 2 && p[1] == ':'
}

func crossDevice(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && en == 17 // ERROR_NOT_SAME_DEVICE
}

func devOf(fs.FileInfo) (uint64, bool)      { return 0, false }
func diskOf(fi fs.FileInfo) int64           { return fi.Size() }
func inodeOf(fs.FileInfo) ([2]uint64, bool) { return [2]uint64{}, false }
func linksOf(fs.FileInfo) uint64            { return 1 }
func shellArgv(_ *Env, script string) []string {
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}
}

const createNoWindow = 0x08000000

func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow}
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// killTree stops a command and what it started (taskkill /T).
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideWindow(k)
	if err := k.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func kill(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return errf(CodeNotFound, "no process %d", pid)
	}
	if err := p.Kill(); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return errf(CodePermission, "process %d needs an administrator to stop", pid)
		}
		return err
	}
	return nil
}

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// processes: the Toolhelp snapshot, and each one's memory and file where
// this person may read them. (No CPU: Windows has no instant figure.)
func processes(_ context.Context, _ *Env) ([]Process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []Process
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := Process{PID: int(e.ProcessID), PPID: int(e.ParentProcessID), Name: windows.UTF16ToString(e.ExeFile[:])}
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, e.ProcessID); err == nil {
			var mc processMemoryCounters
			mc.Cb = uint32(unsafe.Sizeof(mc))
			if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.Cb)); r != 0 {
				p.Memory = uint64(mc.WorkingSetSize)
			}
			buf := make([]uint16, windows.MAX_LONG_PATH)
			n := uint32(len(buf))
			if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
				p.Path = windows.UTF16ToString(buf[:n])
			}
			windows.CloseHandle(h)
		}
		out = append(out, p)
	}
	return out, nil
}

// ps runs a PowerShell script, its output in UTF-8.
func ps(ctx context.Context, env *Env, stdin []byte, script string) ([]byte, error) {
	script = "[Console]::OutputEncoding=[Text.Encoding]::UTF8; [Console]::InputEncoding=[Text.Encoding]::UTF8; $ErrorActionPreference='Stop'; " + script
	return runOut(ctx, env, stdin, nil, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

// trash: the recycle bin, through .NET's FileSystem (as Explorer's delete).
func trash(ctx context.Context, env *Env, paths []string) []error {
	errs := make([]error, len(paths))
	in, _ := json.Marshal(paths)
	out, err := ps(ctx, env, in, `Add-Type -AssemblyName Microsoft.VisualBasic
$paths = [Console]::In.ReadToEnd() | ConvertFrom-Json
foreach ($p in @($paths)) {
  try {
    if (Test-Path -LiteralPath $p -PathType Container) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin') }
    else { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin') }
    Write-Output "ok"
  } catch { Write-Output ("err " + $_.Exception.Message) }
}`)
	if err != nil {
		for i := range errs {
			errs[i] = err
		}
		return errs
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(out)), "\r", ""), "\n")
	for i := range paths {
		switch {
		case i >= len(lines):
			errs[i] = errf(CodeFailed, "no answer for %s", paths[i])
		case strings.HasPrefix(lines[i], "err "):
			errs[i] = errf(CodeFailed, "%s", strings.TrimPrefix(lines[i], "err "))
		}
	}
	return errs
}

func reveal(_ *Env, p string) error {
	cmd := exec.Command("explorer.exe", "/select,", p)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // (explorer exits 1 even when it worked)
	return nil
}

func open(env *Env, p, app string) error {
	if app != "" {
		prog, err := env.LookPath(app)
		if err != nil {
			return errf(CodeNotFound, "%s is not installed here (not on PATH)", app)
		}
		cmd := exec.Command(prog, p)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", p)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func clipboardMissing(*Env) string { return "" }

func clipboardGet(ctx context.Context, env *Env) (string, error) {
	out, err := ps(ctx, env, nil, `$t = Get-Clipboard -Raw; if ($t) { [Console]::Out.Write($t) }`)
	return string(out), err
}

func clipboardSet(ctx context.Context, env *Env, s string) error {
	if s == "" {
		_, err := ps(ctx, env, nil, `Set-Clipboard -Value $null`)
		return err
	}
	_, err := ps(ctx, env, []byte(s), `Set-Clipboard -Value ([Console]::In.ReadToEnd())`)
	return err
}

func notifyMissing(*Env) string { return "" }

// notify: a toast, under PowerShell's own app id (a program needs one
// registered to show toasts; PowerShell's always is).
func notify(ctx context.Context, env *Env, title, subtitle, body string, sound bool) error {
	if title == "" {
		title = "myco"
	}
	in, _ := json.Marshal(map[string]any{"title": title, "subtitle": subtitle, "body": body, "silent": !sound})
	_, err := ps(ctx, env, in, `$n = [Console]::In.ReadToEnd() | ConvertFrom-Json
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$esc = { param($s) [Security.SecurityElement]::Escape([string]$s) }
$lines = "<text>" + (& $esc $n.title) + "</text>"
if ($n.subtitle) { $lines += "<text>" + (& $esc $n.subtitle) + "</text>" }
if ($n.body) { $lines += "<text>" + (& $esc $n.body) + "</text>" }
$audio = if ($n.silent) { '<audio silent="true"/>' } else { '' }
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml("<toast><visual><binding template=""ToastGeneric"">$lines</binding></visual>$audio</toast>")
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($xml))`)
	return err
}

func screenMissing(*Env) string { return "" }

// capture: the whole desktop (every display) or one display, at its real
// pixels (the script asks to be DPI aware first). A region is cut out
// afterwards.
func capture(ctx context.Context, env *Env, file string, display int, _ *Region) (bool, error) {
	in, _ := json.Marshal(map[string]any{"file": file, "display": display})
	_, err := ps(ctx, env, in, `$a = [Console]::In.ReadToEnd() | ConvertFrom-Json
Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public static class MycoDpi { [DllImport("user32.dll")] public static extern bool SetProcessDPIAware(); }'
[MycoDpi]::SetProcessDPIAware() | Out-Null
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
if ($a.display -gt 0) {
  $screens = [System.Windows.Forms.Screen]::AllScreens | Sort-Object { -not $_.Primary }
  if ($a.display -gt $screens.Count) { throw "there is no display $($a.display)" }
  $b = $screens[$a.display - 1].Bounds
} else { $b = [System.Windows.Forms.SystemInformation]::VirtualScreen }
$bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($b.Left, $b.Top, 0, 0, $bmp.Size)
$bmp.Save($a.file, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()`)
	return false, err
}

// exeNames: p with each of PATHEXT's endings (.exe, .cmd, …).
func exeNames(p string) []string {
	if filepath.Ext(p) != "" {
		return []string{p}
	}
	exts := strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";")
	if len(exts) == 1 && exts[0] == "" {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}
	out := []string{}
	for _, e := range exts {
		if e != "" {
			out = append(out, p+e)
		}
	}
	return out
}

var procGetSystemTimes = kernel32.NewProc("GetSystemTimes")

func systemTimes() (idle, total uint64, ok bool) {
	var i, k, u windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return 0, 0, false
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	// (kernel time includes idle time)
	return ft(i), ft(k) + ft(u), true
}

// usage: GetSystemTimes sampled twice, GlobalMemoryStatusEx.
func usage(ctx context.Context, _ *Env) (Usage, error) {
	u := Usage{}
	if i1, t1, ok := systemTimes(); ok {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return u, ctx.Err()
		}
		if i2, t2, ok := systemTimes(); ok && t2 > t1 {
			u.CPU = 100 * (1 - float64(i2-i1)/float64(t2-t1))
		}
	}
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		u.MemoryTotal, u.MemoryAvailable = m.TotalPhys, m.AvailPhys
		u.MemoryUsed = m.TotalPhys - m.AvailPhys
	}
	return u, nil
}

// apps: what Settings › Apps lists (the Uninstall keys, for the machine
// and for this person).
func apps(ctx context.Context, _ *Env) ([]AppInfo, error) {
	var out []AppInfo
	seen := map[string]bool{}
	for _, src := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	} {
		k, err := registry.OpenKey(src.root, src.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			sk, err := registry.OpenKey(src.root, src.path+`\`+n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			str := func(name string) string { v, _, _ := sk.GetStringValue(name); return v }
			a := AppInfo{Name: str("DisplayName"), ID: n, Version: str("DisplayVersion"), Publisher: str("Publisher"), Path: str("InstallLocation")}
			if kb, _, err := sk.GetIntegerValue("EstimatedSize"); err == nil {
				a.Size = int64(kb) * 1024
			}
			sysComp, _, _ := sk.GetIntegerValue("SystemComponent")
			parent := str("ParentKeyName")
			sk.Close()
			if a.Name == "" || sysComp == 1 || parent != "" || seen[a.Name+a.Version] {
				continue
			}
			seen[a.Name+a.Version] = true
			out = append(out, a)
		}
	}
	return out, nil
}

var settingsURIs = map[string]string{
	"full_disk": "ms-settings:privacy-broadfilesystemaccess", "screen_recording": "ms-settings:privacy-graphicsCaptureProgrammatic",
	"accessibility": "ms-settings:easeofaccess", "privacy": "ms-settings:privacy", "notifications": "ms-settings:notifications",
	"storage": "ms-settings:storagesense", "network": "ms-settings:network-status", "display": "ms-settings:display",
	"sound": "ms-settings:sound", "bluetooth": "ms-settings:bluetooth",
}

func openSettings(_ *Env, pane string) error {
	cmd := exec.Command("explorer.exe", settingsURIs[pane])
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // (explorer exits 1 even when it worked)
	return nil
}

func thumbnail(context.Context, *Env, string, int) ([]byte, error) {
	return nil, errf(CodeUnsupported, "on Windows only pictures (png, jpeg, gif) have thumbnails for now")
}

func dialogMissing(*Env) string { return "" }

// dialog: Windows Forms' dialogs, on top, from PowerShell.
func dialog(ctx context.Context, env *Env, d Dialog) ([]string, error) {
	var filter string
	if len(d.Types) > 0 {
		var pats []string
		for _, t := range d.Types {
			pats = append(pats, "*."+strings.TrimPrefix(t, "."))
		}
		filter = strings.Join(pats, ";")
	}
	in, _ := json.Marshal(map[string]any{"title": d.Title, "multiple": d.Multiple, "folders": d.Folders, "save": d.save, "name": d.Name, "folder": d.Folder, "filter": filter})
	script := `[Console]::OutputEncoding=[Text.Encoding]::UTF8; [Console]::InputEncoding=[Text.Encoding]::UTF8; $ErrorActionPreference='Stop'
$a = [Console]::In.ReadToEnd() | ConvertFrom-Json
Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true; ShowInTaskbar = $false; WindowState = 'Minimized' }
if ($a.folders) {
  $dlg = New-Object System.Windows.Forms.FolderBrowserDialog
  if ($a.title) { $dlg.Description = $a.title }
  if ($a.folder) { $dlg.SelectedPath = $a.folder }
  if ($dlg.ShowDialog($owner) -eq 'OK') { $dlg.SelectedPath }
} elseif ($a.save) {
  $dlg = New-Object System.Windows.Forms.SaveFileDialog
  if ($a.title) { $dlg.Title = $a.title }
  if ($a.folder) { $dlg.InitialDirectory = $a.folder }
  if ($a.name) { $dlg.FileName = $a.name }
  if ($a.filter) { $dlg.Filter = "$($a.filter)|$($a.filter)" }
  if ($dlg.ShowDialog($owner) -eq 'OK') { $dlg.FileName }
} else {
  $dlg = New-Object System.Windows.Forms.OpenFileDialog
  if ($a.title) { $dlg.Title = $a.title }
  if ($a.folder) { $dlg.InitialDirectory = $a.folder }
  if ($a.filter) { $dlg.Filter = "$($a.filter)|$($a.filter)" }
  $dlg.Multiselect = [bool]$a.multiple
  if ($dlg.ShowDialog($owner) -eq 'OK') { $dlg.FileNames }
}`
	out, err := runOutFor(ctx, env, 30*time.Minute, in, nil, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, l := range strings.Split(strings.ReplaceAll(strings.TrimSpace(string(out)), "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}
