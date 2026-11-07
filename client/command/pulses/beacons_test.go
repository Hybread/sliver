package pulses

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/bishopfox/sliver/client/assets"
	"github.com/bishopfox/sliver/client/command/targettable"
	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/protobuf/clientpb"
)

var pulseTableTestNow = time.Date(2026, time.September, 3, 21, 25, 41, 0, time.UTC)

func TestRenderPulsesUsesFullProcessPathWhenItFits(t *testing.T) {
	pulse := testWindowsPulse()
	layout := targettable.Layout{IncludeIntegrity: true, IncludeLocale: true}
	width := pulseCandidateWidth(t, []*clientpb.Pulse{pulse}, layout, true)

	rendered := renderPulseTableAtWidth(t, []*clientpb.Pulse{pulse}, "", nil, width)
	if !strings.Contains(rendered, pulse.Filename+" (2020)") {
		t.Fatalf("full process path missing from table:\n%s", rendered)
	}
	if !strings.Contains(rendered, "(36s ago)") || !strings.Contains(rendered, "(in 24s)") {
		t.Fatalf("wide check-in timestamps changed unexpectedly:\n%s", rendered)
	}
	assertPulseTableFits(t, rendered, width)
}

func TestRenderPulsesUsesPortableProcessBasenameWhenRequired(t *testing.T) {
	tests := []struct {
		name         string
		pulse       *clientpb.Pulse
		hasIntegrity bool
		wantProcess  string
	}{
		{
			name:         "windows path",
			pulse:       testWindowsPulse(),
			hasIntegrity: true,
			wantProcess:  "winterfell.exe (2020)",
		},
		{
			name:         "posix path",
			pulse:       testLinuxPulse(),
			hasIntegrity: false,
			wantProcess:  "winterfell-agent (2020)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pulses := []*clientpb.Pulse{tt.pulse}
			fullLayout := targettable.Layout{
				IncludeIntegrity: tt.hasIntegrity,
				IncludeLocale:    true,
			}
			basenameLayout := fullLayout
			basenameLayout.ProcessBasename = true
			fullWidth := pulseCandidateWidth(t, pulses, fullLayout, tt.hasIntegrity)
			basenameWidth := pulseCandidateWidth(t, pulses, basenameLayout, tt.hasIntegrity)
			if basenameWidth >= fullWidth {
				t.Fatalf("basename candidate width %d is not narrower than full-path width %d", basenameWidth, fullWidth)
			}

			rendered := renderPulseTableAtWidth(t, pulses, "", nil, basenameWidth)
			if !strings.Contains(rendered, tt.wantProcess) {
				t.Fatalf("process basename missing from table:\n%s", rendered)
			}
			if strings.Contains(rendered, tt.pulse.Filename) {
				t.Fatalf("full process path remained in basename layout:\n%s", rendered)
			}
			assertPulseTableFits(t, rendered, basenameWidth)
		})
	}
}

func TestRenderPulsesOmitsIntegrityBeforeLocale(t *testing.T) {
	pulse := testWindowsPulse()
	pulses := []*clientpb.Pulse{pulse}
	withAll := targettable.Layout{
		ProcessBasename:  true,
		IncludeIntegrity: true,
		IncludeLocale:    true,
	}
	withoutIntegrity := withAll
	withoutIntegrity.IncludeIntegrity = false
	withoutLocale := withoutIntegrity
	withoutLocale.IncludeLocale = false

	allWidth := pulseCandidateWidth(t, pulses, withAll, true)
	withoutIntegrityWidth := pulseCandidateWidth(t, pulses, withoutIntegrity, true)
	withoutLocaleWidth := pulseCandidateWidth(t, pulses, withoutLocale, true)
	if !(withoutLocaleWidth < withoutIntegrityWidth && withoutIntegrityWidth < allWidth) {
		t.Fatalf("candidate widths did not shrink in omission order: all=%d no-integrity=%d no-locale=%d", allWidth, withoutIntegrityWidth, withoutLocaleWidth)
	}

	withoutIntegrityRendered := renderPulseTableAtWidth(t, pulses, "", nil, withoutIntegrityWidth)
	assertHeaderPresence(t, withoutIntegrityRendered, "Integrity", false)
	assertHeaderPresence(t, withoutIntegrityRendered, "Locale", true)
	assertPulseTableFits(t, withoutIntegrityRendered, withoutIntegrityWidth)

	withoutLocaleRendered := renderPulseTableAtWidth(t, pulses, "", nil, withoutLocaleWidth)
	assertHeaderPresence(t, withoutLocaleRendered, "Integrity", false)
	assertHeaderPresence(t, withoutLocaleRendered, "Locale", false)
	assertPulseTableFits(t, withoutLocaleRendered, withoutLocaleWidth)
}

func TestRenderPulsesDoesNotAddIntegrityForNonWindowsTargets(t *testing.T) {
	pulse := testLinuxPulse()
	pulses := []*clientpb.Pulse{pulse}
	layout := targettable.Layout{IncludeLocale: true}
	width := pulseCandidateWidth(t, pulses, layout, false)

	rendered := renderPulseTableAtWidth(t, pulses, "", nil, width)
	assertHeaderPresence(t, rendered, "Integrity", false)
	assertHeaderPresence(t, rendered, "Locale", true)
	if !strings.Contains(rendered, pulse.Filename+" (2020)") {
		t.Fatalf("non-Windows full process path missing from table:\n%s", rendered)
	}
	assertPulseTableFits(t, rendered, width)
}

func TestRenderPulsesFiltersCanonicalFullRows(t *testing.T) {
	pulse := testWindowsPulse()
	pulses := []*clientpb.Pulse{pulse}
	basenameLayout := targettable.Layout{
		ProcessBasename:  true,
		IncludeIntegrity: true,
		IncludeLocale:    true,
	}
	basenameWidth := pulseCandidateWidth(t, pulses, basenameLayout, true)
	withoutLocale := basenameLayout
	withoutLocale.IncludeIntegrity = false
	withoutLocale.IncludeLocale = false
	withoutLocaleWidth := pulseCandidateWidth(t, pulses, withoutLocale, true)

	t.Run("full process path survives basename projection", func(t *testing.T) {
		rendered := renderPulseTableAtWidth(t, pulses, `AppData\Local\Temp`, nil, basenameWidth)
		if !strings.Contains(rendered, pulse.Name) {
			t.Fatalf("filter matching canonical process path removed pulse:\n%s", rendered)
		}
		if strings.Contains(rendered, `AppData\Local\Temp`) {
			t.Fatalf("basename layout unexpectedly displayed filtered directory:\n%s", rendered)
		}
	})

	t.Run("hidden integrity survives column projection", func(t *testing.T) {
		rendered := renderPulseTableAtWidth(t, pulses, "", regexp.MustCompile(`^High$`), withoutLocaleWidth)
		if !strings.Contains(rendered, pulse.Name) {
			t.Fatalf("filter matching hidden integrity removed pulse:\n%s", rendered)
		}
		assertHeaderPresence(t, rendered, "Integrity", false)
	})
}

func TestRenderPulsesDoesNotMutateEmptyIntegrity(t *testing.T) {
	pulse := testWindowsPulse()
	pulse.Integrity = ""
	layout := targettable.Layout{IncludeIntegrity: true, IncludeLocale: true}
	width := pulseCandidateWidth(t, []*clientpb.Pulse{pulse}, layout, true)

	_ = renderPulseTableAtWidth(t, []*clientpb.Pulse{pulse}, "", nil, width)
	if pulse.Integrity != "" {
		t.Fatalf("pulse integrity mutated to %q", pulse.Integrity)
	}
}

func TestRenderPulsesCompactFallbackPreservesLegacyColumnsAndTimes(t *testing.T) {
	pulse := testWindowsPulse()
	pulses := []*clientpb.Pulse{pulse}
	compactWidth := pulseCandidateWidth(t, pulses, targettable.Layout{Compact: true}, true)
	withoutOptionalWidth := pulseCandidateWidth(t, pulses, targettable.Layout{ProcessBasename: true}, true)
	if compactWidth >= withoutOptionalWidth {
		t.Fatalf("test fixture does not require compact fallback: compact=%d detailed=%d", compactWidth, withoutOptionalWidth)
	}
	rendered := renderPulseTableAtWidth(t, pulses, "", nil, compactWidth)

	for _, omitted := range []string{"Tasks", "Remote Address", "Process (PID)", "Integrity", "Locale"} {
		assertHeaderPresence(t, rendered, omitted, false)
	}
	if !strings.Contains(rendered, "36s") || !strings.Contains(rendered, "24s") {
		t.Fatalf("compact relative check-in times missing:\n%s", rendered)
	}
	if strings.Contains(rendered, "36s ago") || strings.Contains(rendered, "in 24s") {
		t.Fatalf("compact fallback did not preserve legacy timestamp format:\n%s", rendered)
	}
	assertPulseTableFits(t, rendered, compactWidth)
}

func testWindowsPulse() *clientpb.Pulse {
	return &clientpb.Pulse{
		ID:                  "0c9f6b27-1111-2222-3333-444444444444",
		Name:                "winterfell-session",
		TasksCount:          3,
		TasksCountCompleted: 2,
		Transport:           "mtls",
		RemoteAddress:       "10.13.37.11:49760",
		Hostname:            "winterfell",
		Username:            `winterfell\vagrant`,
		Filename:            `C:\Users\vagrant\AppData\Local\Temp\sliver-range\winterfell.exe`,
		PID:                 2020,
		Integrity:           "High",
		OS:                  "windows",
		Arch:                "amd64",
		Locale:              "en-US",
		LastCheckin:         pulseTableTestNow.Add(-36 * time.Second).Unix(),
		NextCheckin:         pulseTableTestNow.Add(24 * time.Second).Unix(),
	}
}

func testLinuxPulse() *clientpb.Pulse {
	pulse := testWindowsPulse()
	pulse.OS = "linux"
	pulse.Hostname = "winterfell-linux"
	pulse.Username = "vagrant"
	pulse.Filename = "/opt/sliver/temporary/build/output/winterfell-agent"
	pulse.Integrity = ""
	return pulse
}

func testPulseConsole() *console.SliverClient {
	return &console.SliverClient{
		Settings: &assets.ClientSettings{TableStyle: "SliverDefault"},
	}
}

func pulseCandidateWidth(t *testing.T, pulses []*clientpb.Pulse, layout targettable.Layout, hasIntegrity bool) int {
	t.Helper()
	tw := buildPulseTable(pulses, "", nil, testPulseConsole(), layout, hasIntegrity, pulseTableTestNow)
	return text.LongestLineLen(tw.Render())
}

func renderPulseTableAtWidth(t *testing.T, pulses []*clientpb.Pulse, filter string, filterRegex *regexp.Regexp, width int) string {
	t.Helper()
	tw := renderPulsesAtWidth(pulses, filter, filterRegex, testPulseConsole(), width, pulseTableTestNow)
	return text.StripEscape(tw.Render())
}

func assertPulseTableFits(t *testing.T, rendered string, width int) {
	t.Helper()
	if got := text.LongestLineLen(rendered); got > width {
		t.Fatalf("rendered table width %d exceeds terminal width %d:\n%s", got, width, rendered)
	}
}

func assertHeaderPresence(t *testing.T, rendered string, header string, want bool) {
	t.Helper()
	headerLine := strings.SplitN(rendered, "\n", 2)[0]
	if got := strings.Contains(strings.ToLower(headerLine), strings.ToLower(header)); got != want {
		t.Fatalf("header %q presence = %v, want %v; header line: %s", header, got, want, headerLine)
	}
}
