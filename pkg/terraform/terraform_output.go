package terraform

import (
	"bytes"
	"io"
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

type lineWriter struct {
	out       io.Writer
	redactor  *secretRedactor
	output    bytes.Buffer
	shownUpTo int
	failed    bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	scanned := w.output.Len()
	w.output.Write(p)
	for {
		end := bytes.IndexByte(w.output.Bytes()[scanned:], '\n')
		if end < 0 {
			return len(p), nil
		}
		scanned += end + 1
		w.show(w.output.Bytes()[w.shownUpTo:scanned])
		w.shownUpTo = scanned
	}
}

func (w *lineWriter) flush() {
	if w.shownUpTo < w.output.Len() {
		w.show(w.output.Bytes()[w.shownUpTo:])
		w.shownUpTo = w.output.Len()
	}
}

func (w *lineWriter) show(line []byte) {
	if w.failed {
		return
	}
	if _, err := io.WriteString(w.out, w.redactor.redact(string(line))); err != nil {
		w.failed = true
	}
}
