package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var excludeDirs = map[string]bool{
	"vendor":     true,
	".git":       true,
	"docs":       true,
	"tools":      true,
	".github":    true,
}

var excludePaths = []string{
	filepath.Join("implant", "vendor"),
}

var includeRoots = []string{
	"protobuf",
	"implant",
	"server",
	"client",
	"test",
}

var replacements = []struct {
	old string
	new string
}{
	{"Beacon", "Pulse"},
	{"beacon", "pulse"},
	{"BEACON", "PULSE"},
}

var fileRenames = []struct {
	old string
	new string
}{
	{filepath.Join("implant", "sliver", "transports", "beacon.go"), filepath.Join("implant", "sliver", "transports", "pulse.go")},
	{filepath.Join("implant", "sliver", "transports", "beacon_yamux_test.go"), filepath.Join("implant", "sliver", "transports", "pulse_yamux_test.go")},
	{filepath.Join("implant", "sliver", "runner", "runner_beacon_test.go"), filepath.Join("implant", "sliver", "runner", "runner_pulse_test.go")},
	{filepath.Join("server", "db", "models", "beacon.go"), filepath.Join("server", "db", "models", "pulse.go")},
	{filepath.Join("server", "db", "models", "beacon_test.go"), filepath.Join("server", "db", "models", "pulse_test.go")},
	{filepath.Join("server", "handlers", "beacons.go"), filepath.Join("server", "handlers", "pulses.go")},
	{filepath.Join("server", "handlers", "beacons_test.go"), filepath.Join("server", "handlers", "pulses_test.go")},
	{filepath.Join("server", "rpc", "rpc-beacons.go"), filepath.Join("server", "rpc", "rpc-pulses.go")},
	{filepath.Join("server", "c2", "mtls_yamux_beacon_e2e_test.go"), filepath.Join("server", "c2", "mtls_yamux_pulse_e2e_test.go")},
	{filepath.Join("server", "c2", "wireguard_yamux_beacon_e2e_test.go"), filepath.Join("server", "c2", "wireguard_yamux_pulse_e2e_test.go")},
	{filepath.Join("client", "command", "use", "beacons.go"), filepath.Join("client", "command", "use", "pulses.go")},
	{filepath.Join("client", "command", "generate", "generate-beacon.go"), filepath.Join("client", "command", "generate", "generate-pulse.go")},
	{filepath.Join("client", "command", "settings", "beacons.go"), filepath.Join("client", "command", "settings", "pulses.go")},
	{filepath.Join("client", "forms", "generate_beacon.go"), filepath.Join("client", "forms", "generate_pulse.go")},
	{filepath.Join("client", "forms", "generate_profiles_new_beacon.go"), filepath.Join("client", "forms", "generate_profiles_new_pulse.go")},
}

var dirRenames = []struct {
	old string
	new string
}{
	{filepath.Join("client", "command", "beacons"), filepath.Join("client", "command", "pulses")},
}

type stats struct {
	filesScanned   int
	filesModified  int
	filesRenamed   int
	dirsRenamed    int
	replacements   int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <sliver-root> [--dry-run]\n", os.Args[0])
		os.Exit(1)
	}

	root := os.Args[1]
	dryRun := len(os.Args) > 2 && os.Args[2] == "--dry-run"

	if dryRun {
		fmt.Println("=== DRY RUN MODE ===")
	}

	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s does not look like a Sliver root (no go.mod)\n", root)
		os.Exit(1)
	}

	s := &stats{}

	fmt.Println("Phase 1: Content replacement")
	for _, includeRoot := range includeRoots {
		dir := filepath.Join(root, includeRoot)
		if _, err := os.Stat(dir); err != nil {
			fmt.Printf("  SKIP (not found): %s\n", dir)
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, _ := filepath.Rel(root, path)

			if info.IsDir() {
				if excludeDirs[info.Name()] {
					return filepath.SkipDir
				}
				for _, ep := range excludePaths {
					if strings.HasPrefix(relPath, ep) {
						return filepath.SkipDir
					}
				}
				return nil
			}

			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".proto" {
				return nil
			}

			return processFile(path, relPath, dryRun, s)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error walking %s: %v\n", dir, err)
		}
	}

	fmt.Printf("\nPhase 2: File renames (%d files)\n", len(fileRenames))
	for _, r := range fileRenames {
		oldPath := filepath.Join(root, r.old)
		newPath := filepath.Join(root, r.new)
		if _, err := os.Stat(oldPath); err != nil {
			fmt.Printf("  SKIP (not found): %s\n", r.old)
			continue
		}
		fmt.Printf("  %s -> %s\n", r.old, r.new)
		if !dryRun {
			if err := os.Rename(oldPath, newPath); err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR renaming: %v\n", err)
			} else {
				s.filesRenamed++
			}
		} else {
			s.filesRenamed++
		}
	}

	fmt.Printf("\nPhase 3: Directory renames (%d dirs)\n", len(dirRenames))
	for _, r := range dirRenames {
		oldPath := filepath.Join(root, r.old)
		newPath := filepath.Join(root, r.new)
		if _, err := os.Stat(oldPath); err != nil {
			fmt.Printf("  SKIP (not found): %s\n", r.old)
			continue
		}
		fmt.Printf("  %s -> %s\n", r.old, r.new)
		if !dryRun {
			if err := os.Rename(oldPath, newPath); err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR renaming: %v\n", err)
			} else {
				s.dirsRenamed++
			}
		} else {
			s.dirsRenamed++
		}
	}

	fmt.Printf("\nPhase 4: Update import paths for renamed packages\n")
	if !dryRun {
		updateImportPaths(root, s)
	}

	fmt.Println("\n=== SUMMARY ===")
	fmt.Printf("Files scanned:  %d\n", s.filesScanned)
	fmt.Printf("Files modified: %d\n", s.filesModified)
	fmt.Printf("Files renamed:  %d\n", s.filesRenamed)
	fmt.Printf("Dirs renamed:   %d\n", s.dirsRenamed)
	fmt.Printf("Replacements:   %d\n", s.replacements)
	if dryRun {
		fmt.Println("\n(Dry run — no changes written)")
	}
}

func processFile(path string, relPath string, dryRun bool, s *stats) error {
	s.filesScanned++

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", relPath, err)
	}

	original := string(data)
	modified := original

	for _, r := range replacements {
		count := strings.Count(modified, r.old)
		if count > 0 {
			modified = strings.ReplaceAll(modified, r.old, r.new)
			s.replacements += count
		}
	}

	if modified != original {
		s.filesModified++
		if !dryRun {
			if err := os.WriteFile(path, []byte(modified), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", relPath, err)
			}
		}
		count := 0
		for _, r := range replacements {
			count += strings.Count(original, r.old)
		}
		fmt.Printf("  %s (%d replacements)\n", relPath, count)
	}

	return nil
}

func updateImportPaths(root string, s *stats) {
	oldImport := `"github.com/bishopfox/sliver/client/command/beacons"`
	newImport := `"github.com/bishopfox/sliver/client/command/pulses"`

	for _, includeRoot := range includeRoots {
		dir := filepath.Join(root, includeRoot)
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}

			relPath, _ := filepath.Rel(root, path)
			for _, ep := range excludePaths {
				if strings.HasPrefix(relPath, ep) {
					return nil
				}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			content := string(data)
			if strings.Contains(content, oldImport) {
				content = strings.ReplaceAll(content, oldImport, newImport)
				os.WriteFile(path, []byte(content), 0644)
				fmt.Printf("  Updated import in %s\n", relPath)
				s.replacements++
			}

			return nil
		})
	}
}
