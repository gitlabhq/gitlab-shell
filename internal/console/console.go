package console

import (
	"fmt"
	"io"
	"strings"
)

func DisplayWarningMessage(message string, out io.Writer) {
	DisplayWarningMessages([]string{message}, out)
}

func DisplayInfoMessage(message string, out io.Writer) {
	DisplayInfoMessages([]string{message}, out)
}

func DisplayWarningMessages(messages []string, out io.Writer) {
	DisplayMessages(messages, out, true)
}

func DisplayInfoMessages(messages []string, out io.Writer) {
	DisplayMessages(messages, out, false)
}

func DisplayMessages(messages []string, out io.Writer, displayDivider bool) {
	if noMessages(messages) {
		return
	}

	displayBlankLineOrDivider(out, displayDivider)

	for _, msg := range messages {
		fmt.Fprintf(out, formatLine(msg))
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
		fmt.Fprintf(out, divider())
	} else {
		fmt.Fprintf(out, blankLine())
	}
}

func blankLine() string {
	return formatLine("")
}

func divider() string {
	ruler := strings.Repeat("=", 72)

	return fmt.Sprintf("%v%v%v", blankLine(), formatLine(ruler), blankLine())
}
