package terraform

import (
	"bytes"
	"io"
	"strings"
)

// showsTerraformOutput is false for commands whose output yago reads itself
func showsTerraformOutput(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "output", "graph":
		return false
	default:
		return true
	}
}

const terraformPrompt = "Enter a value:"

// the real coloured prompt is about 30 bytes, this leaves room for indentation and colour codes
const maxPromptLine = 128

type lineWriter struct {
	out       io.Writer
	redactor  *secretRedactor
	output    bytes.Buffer
	shownUpTo int
	lineStart int
	// what's already been shown of the current line, a prompt shown before its line ended
	early  string
	failed bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	scanned := w.output.Len()
	w.output.Write(p)
	for {
		end := bytes.IndexByte(w.output.Bytes()[scanned:], '\n')
		if end < 0 {
			w.showPrompt()
			return len(p), nil
		}
		scanned += end + 1
		w.showUpTo(scanned)
		w.lineStart = scanned
	}
}

// the line is redacted whole once it's complete, so splitting it round a prompt can't change what's hidden
func (w *lineWriter) showUpTo(end int) {
	if w.shownUpTo >= end {
		return
	}
	rest := w.redactor.redact(string(w.output.Bytes()[w.shownUpTo:end]))
	if w.early != "" {
		if whole := w.redactor.redact(string(w.output.Bytes()[w.lineStart:end])); strings.HasPrefix(whole, w.early) {
			rest = whole[len(w.early):]
		}
		w.early = ""
	}
	w.write(rest)
	w.shownUpTo = end
}

// terraform's prompt has no newline and waits for an answer, so it'd never show otherwise, it's
// only shown early when the unfinished line is nothing but the prompt and no secret could start in it
func (w *lineWriter) showPrompt() {
	// a line only grows, so once it's too long to be the prompt it's never checked again, which keeps
	// long lines streamed in chunks linear
	if w.output.Len()-w.shownUpTo > maxPromptLine {
		return
	}
	pending := string(w.output.Bytes()[w.shownUpTo:])
	plain := ansiEscape.ReplaceAllString(pending, "")
	if strings.TrimSpace(plain) != terraformPrompt {
		return
	}
	// secrets are matched on the text with colour codes taken out, so that's what's checked
	if w.redactor.couldContinue(plain) {
		return
	}
	shown := w.redactor.redact(pending)
	w.write(shown)
	w.early += shown
	w.shownUpTo = w.output.Len()
}

func (w *lineWriter) flush() {
	w.showUpTo(w.output.Len())
}

func (w *lineWriter) write(text string) {
	if w.failed || text == "" {
		return
	}
	if _, err := io.WriteString(w.out, text); err != nil {
		w.failed = true
	}
}

// couldContinue is true when text holds a secret or ends in the start of one, so showing it now
// could leak what comes next
func (r *secretRedactor) couldContinue(text string) bool {
	if r.redact(text) != text {
		return true
	}
	endsInStartOf := func(secret string) bool {
		for n := min(len(secret), len(text)); n > 0; n-- {
			if strings.HasSuffix(text, secret[:n]) {
				return true
			}
		}
		return false
	}
	for _, value := range r.values {
		if endsInStartOf(value) {
			return true
		}
	}
	for line := range r.lines {
		if endsInStartOf(line) {
			return true
		}
	}
	return false
}
