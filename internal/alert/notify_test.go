package alert

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestNotifyCommand(t *testing.T) {
	// Quotes, a backslash, a newline and a leading dash must all arrive
	// intact.
	const (
		title = `Focus "done"`
		body  = "-it's 5 o'clock \\ again\nreally"
	)
	tests := []struct {
		name    string
		goos    string
		tools   []string
		want    call
		wantErr error
	}{
		{
			name: "darwin osascript",
			goos: "darwin",
			want: call{"osascript", []string{
				"-e", "on run argv",
				"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
				"-e", "end run",
				"--", title, body,
			}},
		},
		{
			name:  "linux notify-send",
			goos:  "linux",
			tools: []string{"notify-send", "gdbus"},
			want:  call{"notify-send", []string{"-a", "tomomoro", "--", title, body}},
		},
		{
			name:  "linux gdbus fallback",
			goos:  "linux",
			tools: []string{"gdbus"},
			want: call{"gdbus", []string{
				"call", "--session",
				"--dest", "org.freedesktop.Notifications",
				"--object-path", "/org/freedesktop/Notifications",
				"--method", "org.freedesktop.Notifications.Notify",
				`'tomomoro'`,
				"uint32 0",
				`''`,
				`'Focus "done"'`,
				`'-it\'s 5 o\'clock \\ again\nreally'`,
				"@as []",
				"@a{sv} {}",
				"int32 -1",
			}},
		},
		{name: "linux without notifier", goos: "linux", wantErr: exec.ErrNotFound},
		{name: "windows", goos: "windows", tools: []string{"notify-send"}, wantErr: errors.ErrUnsupported},
		{name: "freebsd", goos: "freebsd", tools: []string{"notify-send", "gdbus"}, wantErr: errors.ErrUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r recorder
			fakeOS(t, tt.goos, tt.tools, r.run)

			err := Notify(t.Context(), title, body)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Notify error = %v, want %v", err, tt.wantErr)
				}
				if len(r.calls) != 0 {
					t.Errorf("ran %v despite the error", r.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Notify: %v", err)
			}
			if len(r.calls) != 1 {
				t.Fatalf("ran %d commands, want 1: %v", len(r.calls), r.calls)
			}
			got := r.calls[0]
			if got.name != tt.want.name || !slices.Equal(got.args, tt.want.args) {
				t.Errorf("ran\n  %s %q\nwant\n  %s %q", got.name, got.args, tt.want.name, tt.want.args)
			}
		})
	}
}

func TestUnsupportedErrorNamesOS(t *testing.T) {
	fakeOS(t, "plan9", nil, (&recorder{}).run)
	err := Notify(t.Context(), "t", "b")
	if err == nil || !strings.Contains(err.Error(), "plan9") || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Notify on plan9 = %v, want an unsupported error naming the OS", err)
	}
}

func TestGVariantString(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", `''`},
		{"plain", "Focus done", `'Focus done'`},
		{"single quote", "it's", `'it\'s'`},
		{"double quote", `say "hi"`, `'say "hi"'`},
		{"backslash", `C:\tmp`, `'C:\\tmp'`},
		{"escaped quote stays escaped", `\'`, `'\\\''`},
		{"newline", "a\nb", `'a\nb'`},
		{"unicode", "Pause ☕ 5 min", `'Pause ☕ 5 min'`},
		{"invalid utf-8", "caf\xe9", "'caf\uFFFD'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gvariantString(tt.in); got != tt.want {
				t.Errorf("gvariantString(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
