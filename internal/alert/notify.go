package alert

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Notify shows a desktop notification. Title and body are passed to the OS
// tool as arguments or safely escaped, never interpolated into a shell or
// script unescaped.
func Notify(ctx context.Context, title, body string) error {
	name, args, err := notifyCommand(title, body)
	if err != nil {
		return err
	}
	return runCmd(ctx, name, args...)
}

// notifyCommand returns the program and arguments that show a notification
// on goos.
func notifyCommand(title, body string) (string, []string, error) {
	switch goos {
	case "darwin":
		// The script reads title and body from argv, so neither is ever
		// parsed as AppleScript. osascript drops the "--".
		return "osascript", []string{
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run",
			"--", title, body,
		}, nil
	case "linux":
		if _, err := lookPath("notify-send"); err == nil {
			// "--" keeps a title or body starting with "-" from reading as a flag.
			return "notify-send", []string{"-a", "tomomoro", "--", title, body}, nil
		}
		if _, err := lookPath("gdbus"); err == nil {
			return "gdbus", gdbusNotifyArgs(title, body), nil
		}
		return "", nil, fmt.Errorf("alert: no notifier (tried notify-send, gdbus): %w", exec.ErrNotFound)
	}
	return "", nil, unsupported()
}

// gdbusNotifyArgs builds a gdbus call of org.freedesktop.Notifications.Notify,
// whose signature is susssasa{sv}i. Every argument is a typed GVariant
// literal, so it parses the same whether or not gdbus can introspect the
// service, and none starts with "-".
func gdbusNotifyArgs(title, body string) []string {
	return []string{
		"call", "--session",
		"--dest", "org.freedesktop.Notifications",
		"--object-path", "/org/freedesktop/Notifications",
		"--method", "org.freedesktop.Notifications.Notify",
		gvariantString("tomomoro"), // app_name
		"uint32 0",                 // replaces_id: a new notification
		gvariantString(""),         // app_icon
		gvariantString(title),      // summary
		gvariantString(body),       // body
		"@as []",                   // actions
		"@a{sv} {}",                // hints
		"int32 -1",                 // expire_timeout: the server's default
	}
}

// gvariantString quotes s as a GVariant text-format string literal: single
// quotes, with backslashes, quotes and newlines escaped. Invalid UTF-8 is
// replaced, since a GVariant string must be valid UTF-8.
func gvariantString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range strings.ToValidUTF8(s, "�") {
		switch r {
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}
