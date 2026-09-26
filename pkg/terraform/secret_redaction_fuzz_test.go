package terraform

import (
	"bytes"
	"io"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

type failingConsole struct {
	shown  bytes.Buffer
	writes int
}

func (c *failingConsole) Write(p []byte) (int, error) {
	if c.writes == 0 {
		return 0, io.ErrClosedPipe
	}
	c.writes--
	return c.shown.Write(p)
}

// FuzzLineWriter checks chunking doesn't change what's shown or kept, even once the console fails
func FuzzLineWriter(f *testing.F) {
	for _, seed := range []struct{ value, output string }{
		{"example-value", "input=example-value\nnext line\n"},
		{"pin\n123", "  + pin\n\x1b[32m+\x1b[0m\x1b[0m 123\nno newline at the end"},
		{"+ a\n- b", "input = <<-EOT\n      + a\n      - b\nEOT\n"},
		{`pa"ss&w<o>rd\1`, "input = \"pa\\\"ss&w<o>rd\\\\1\"\r\n"},
		{"abcd", "abcdabcd\n\n\nab\x1b[0mcd\n"},
	} {
		f.Add(seed.value, seed.output, uint64(1), uint8(255))
		f.Add(seed.value, seed.output, uint64(42), uint8(1))
	}
	f.Fuzz(func(t *testing.T, value, output string, seed uint64, consoleWrites uint8) {
		redactor := newSecretRedactor(map[string]string{"TF_VAR_example": value})
		var lines []string
		for line := range strings.SplitAfterSeq(output, "\n") {
			if line != "" {
				lines = append(lines, redactor.redact(line))
			}
		}

		console := &failingConsole{writes: int(consoleWrites)}
		w := &lineWriter{out: console, redactor: redactor}
		chunks := rand.New(rand.NewPCG(seed, seed))
		for rest := []byte(output); len(rest) > 0; {
			n := 1 + chunks.IntN(len(rest))
			if _, err := w.Write(rest[:n]); err != nil {
				t.Fatal(err)
			}
			rest = rest[n:]
		}
		w.flush()

		if w.output.String() != output {
			t.Fatalf("kept %q, want %q", w.output.String(), output)
		}
		want := strings.Join(lines[:min(len(lines), int(consoleWrites))], "")
		if console.shown.String() != want {
			t.Fatalf("shown %q, want %q", console.shown.String(), want)
		}
	})
}

func FuzzSecretRedactorHidesSecrets(f *testing.F) {
	f.Add("example-value", "input = \"", "\"")
	f.Add(`pa"ss&w<o>rd\1`, "input = ", "")
	f.Add("abab", "ab", "ab")
	f.Fuzz(func(t *testing.T, value, before, after string) {
		// [REDACTED] itself could spell out a value with brackets, or one inside it
		if value == "" || strings.ContainsAny(value+before+after, "\n\x1b") || strings.ContainsAny(value, "[]") ||
			strings.Contains("[REDACTED]", value) {
			t.Skip()
		}
		redactor := newSecretRedactor(map[string]string{"TF_VAR_example": value})
		split := len(value) / 2
		for _, output := range []string{before + value + after, before + value[:split] + "\x1b[0m" + value[split:] + after} {
			if shown := ansiEscape.ReplaceAllString(redactor.redact(output), ""); strings.Contains(shown, value) {
				t.Fatalf("redact(%q) shows the secret: %q", output, shown)
			}
		}
		if strings.IndexFunc(value, func(c rune) bool { return c < ' ' || c > '~' }) < 0 {
			quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
			if shown := redactor.redact(before + `"` + quoted + `"` + after); strings.Contains(shown, quoted) {
				t.Fatalf("quoted secret shown: %q", shown)
			}
		}
	})
}

func BenchmarkSecretRedactor(b *testing.B) {
	redactor := newSecretRedactor(map[string]string{"TF_VAR_example": "example-value", "TF_VAR_lines": "pin\n123"})
	for _, bench := range []struct{ name, line string }{
		{"plain", "  # aws_example.resource will be updated in-place, with no secrets in this line at all\n"},
		{"coloured", "  \x1b[33m~\x1b[0m\x1b[0m resource \"aws_example\" \"resource\" {\x1b[0m with no secrets at all\n"},
	} {
		output := strings.Repeat(bench.line, (1<<20)/len(bench.line))
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(output)))
			for b.Loop() {
				redactor.redact(output)
			}
		})
		b.Run(bench.name+"/streamed", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(output)))
			for b.Loop() {
				w := &lineWriter{out: io.Discard, redactor: redactor}
				for chunk := range slices.Chunk([]byte(output), 4096) {
					_, _ = w.Write(chunk)
				}
				w.flush()
			}
		})
	}
}
