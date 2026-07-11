//go:build windows

package browser

import (
	"path/filepath"
	"testing"
)

func TestFindExecutable(t *testing.T) {
	edgeProgramFilesX86 := filepath.Join(`C:\Program Files (x86)`, `Microsoft\Edge\Application\msedge.exe`)
	edgeProgramFiles := filepath.Join(`C:\Program Files`, `Microsoft\Edge\Application\msedge.exe`)
	edgeLocal := filepath.Join(`C:\Users\test\AppData\Local`, `Microsoft\Edge\Application\msedge.exe`)
	chromeProgramFilesX86 := filepath.Join(`C:\Program Files (x86)`, `Google\Chrome\Application\chrome.exe`)
	chromeProgramFiles := filepath.Join(`C:\Program Files`, `Google\Chrome\Application\chrome.exe`)
	chromeLocal := filepath.Join(`C:\Users\test\AppData\Local`, `Google\Chrome\Application\chrome.exe`)

	tests := []struct {
		name    string
		env     map[string]string
		exists  map[string]bool
		want    string
		wantErr string
	}{
		{
			name: "prefers Edge system over all other candidates",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{
				edgeProgramFilesX86:   true,
				edgeProgramFiles:      true,
				edgeLocal:             true,
				chromeProgramFilesX86: true,
				chromeProgramFiles:    true,
				chromeLocal:           true,
			},
			want: edgeProgramFilesX86,
		},
		{
			name: "prefers second Edge system location over Edge user",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{edgeProgramFiles: true, edgeLocal: true},
			want:   edgeProgramFiles,
		},
		{
			name: "prefers Edge user over Chrome system",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{edgeLocal: true, chromeProgramFilesX86: true},
			want:   edgeLocal,
		},
		{
			name: "prefers first Chrome system location over remaining Chrome candidates",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{chromeProgramFilesX86: true, chromeProgramFiles: true, chromeLocal: true},
			want:   chromeProgramFilesX86,
		},
		{
			name: "finds Chrome in second system location",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{chromeProgramFiles: true, chromeLocal: true},
			want:   chromeProgramFiles,
		},
		{
			name: "finds Chrome in user location",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{chromeLocal: true},
			want:   chromeLocal,
		},
		{
			name: "ignores empty environment variables",
			env: map[string]string{
				"PROGRAMFILES(X86)": "",
				"PROGRAMFILES":      "",
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			exists: map[string]bool{"Microsoft\\Edge\\Application\\msedge.exe": true, chromeLocal: true},
			want:   chromeLocal,
		},
		{
			name: "reports when no browser exists",
			env: map[string]string{
				"PROGRAMFILES(X86)": `C:\Program Files (x86)`,
				"PROGRAMFILES":      `C:\Program Files`,
				"LOCALAPPDATA":      `C:\Users\test\AppData\Local`,
			},
			wantErr: "supported browser not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindExecutable(
				func(key string) string { return tt.env[key] },
				func(path string) bool { return tt.exists[path] },
			)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("FindExecutable() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindExecutable() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("FindExecutable() = %q, want %q", got, tt.want)
			}
		})
	}
}
