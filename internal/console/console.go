// Package console provides functions for displaying console messages.
package console

import (
	"fmt"
	"io"
	"strings"
)

// DisplayWarningMessage displays a warning message to the specified output.
func DisplayWarningMessage(message string, out io.Writer) {
	DisplayWarningMessages([]string{message}, out)
}

// DisplayInfoMessage displays an informational message to the specified output.
func DisplayInfoMessage(message string, out io.Writer) {
	DisplayInfoMessages([]string{message}, out)
}

// DisplayWarningMessages displays multiple warning messages to the specified output.
func DisplayWarningMessages(messages []string, out io.Writer) {
	DisplayMessages(messages, out, true)
}

// DisplayInfoMessages displays multiple informational messages to the specified output.
func DisplayInfoMessages(messages []string, out io.Writer) {
	DisplayMessages(messages, out, false)
}

// DisplayMessages displays multiple messages to the specified output, with an optional divider.
func DisplayMessages(messages []string, out io.Writer, displayDivider bool) {
	if noMessages(messages) {
		return
	}

	displayBlankLineOrDivider(out, displayDivider)

	for _, msg := range messages {
		fmt.Fprint(out, formatLine(msg))
	}

	displayBlankLineOrDivider(out, displayDivider)
}

func noMessages(messages []string) bool {
	if len(messages) == 0 {
		return true
	}

	for _, msg := range messages {
		if len(strings.TrimSpace(msg)) > 0 {
			return false
		}
	}

	return true
}

func formatLine(message string) string {
	return fmt.Sprintf("remote: %v\n", message)
}

func displayBlankLineOrDivider(out io.Writer, displayDivider bool) {
	if displayDivider {
		fmt.Fprint(out, divider())
	} else {
		fmt.Fprint(out, blankLine())
	}
}

func blankLine() string {
	return formatLine("")
}

func divider() string {
	ruler := strings.Repeat("=", 72)

	return fmt.Sprintf("%v%v%v", blankLine(), formatLine(ruler), blankLine())
}
