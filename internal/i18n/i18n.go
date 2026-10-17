// Package i18n holds skd's own text in its two languages. English is the
// key: a message is written in English at the place that shows it (as a
// format string for fmt, when it has values), and the Chinese catalogue
// maps that exact English text to its translation. With English selected
// every function returns its input untouched, so English output is what
// the source says, byte for byte.
//
// Rules the tests enforce (catalog_test.go):
//   - every message used through this package has an entry in the Chinese
//     catalogue, and every catalogue entry is used;
//   - an entry keeps the verbs of its key (%s, %d, ...), so a translation
//     cannot drop or invent a value;
//   - error values and prose in fmt.Errorf / errors.New are made here
//     (Errorf, New), not with fmt and errors directly.
//
// Text that is data (JSON, files skd writes, commit messages, names) is
// never passed through this package: it must not depend on the language.
package i18n

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

// Lang is a language skd can speak.
type Lang uint32

const (
	EN Lang = iota
	ZH
)

// Code is the value of the lang setting that selects l.
func (l Lang) Code() string {
	if l == ZH {
		return "zh"
	}
	return "en"
}

var current atomic.Uint32

// Set makes l the language of everything skd prints from now on.
func Set(l Lang) { current.Store(uint32(l)) }

// Get is the language in use.
func Get() Lang { return Lang(current.Load()) }

// zh is the Chinese catalogue: English message -> Chinese message.
var zh = map[string]string{}

// register adds entries to the Chinese catalogue; a message translated
// twice is a bug, not an override.
func register(entries map[string]string) {
	for k, v := range entries {
		if _, dup := zh[k]; dup {
			panic("i18n: message translated twice: " + k)
		}
		zh[k] = v
	}
}

// Catalog returns a copy of the Chinese catalogue (for the tests).
func Catalog() map[string]string {
	out := make(map[string]string, len(zh))
	for k, v := range zh {
		out[k] = v
	}
	return out
}

// translate is msg in language l; an English or unknown message comes
// back as it is.
func translate(l Lang, msg string) string {
	if l == ZH {
		if s, ok := zh[msg]; ok {
			return s
		}
	}
	return msg
}

// T is msg in the language in use.
func T(msg string) string { return translate(Get(), msg) }

// M marks msg as a message to translate later, with T, where it is
// shown: a table or a package variable is built before the language is
// known. It returns msg unchanged.
func M(msg string) string { return msg }

// Sprintf is fmt.Sprintf of the translated format.
func Sprintf(format string, a ...any) string {
	if false {
		_ = fmt.Sprintf(format, a...) // never runs: makes go vet check the calls of Sprintf like fmt's
	}
	return fmt.Sprintf(T(format), a...)
}

// Fprintf is fmt.Fprintf of the translated format.
func Fprintf(w io.Writer, format string, a ...any) (int, error) {
	if false {
		_, _ = fmt.Fprintf(w, format, a...) // never runs: makes go vet check the calls like fmt's
	}
	return fmt.Fprintf(w, T(format), a...)
}

// Count is the translated "n things": one is used when n is 1 and other
// otherwise, both are formats taking n (for example "%d pack" and "%d
// packs"). Chinese has no plural: it has one entry, for other.
func Count(n int, one, other string) string {
	if Get() != ZH && n == 1 {
		return fmt.Sprintf(one, n)
	}
	return fmt.Sprintf(T(other), n)
}

// An Error is an error whose text depends on the language. It keeps its
// format and values, so the text can be had in another language later
// (English, for what skd writes to files).
type Error struct {
	format string
	args   []any
	wrap   error
	plain  bool // format is a message without verbs (New)
}

// inLang is implemented by every error of this package.
type inLang interface{ in(Lang) string }

func (e *Error) Error() string { return e.in(Get()) }

func (e *Error) in(l Lang) string {
	if e.plain {
		return translate(l, e.format)
	}
	f := strings.ReplaceAll(translate(l, e.format), "%w", "%v")
	args := make([]any, len(e.args))
	for i, a := range e.args {
		args[i] = a
		if x, ok := a.(inLang); ok {
			args[i] = x.in(l)
		}
	}
	return fmt.Sprintf(f, args...)
}

// Unwrap returns the error the %w of the format wrapped.
func (e *Error) Unwrap() error { return e.wrap }

// Errorf is fmt.Errorf with a translated text: it wraps what %w names,
// so errors.Is and errors.As see through it.
func Errorf(format string, a ...any) error {
	if false {
		_ = fmt.Errorf(format, a...) // never runs: makes go vet check the calls like fmt's
	}
	e := &Error{format: format, args: a}
	if strings.Contains(format, "%w") {
		e.wrap = errors.Unwrap(fmt.Errorf(format, a...))
	}
	return e
}

// New is errors.New for a message that is translated: a sentinel can be
// declared before the language is known.
func New(msg string) error { return &Error{format: msg, plain: true} }

// English is err's text in English, whatever language is in use: for
// what skd writes to a file, which must not depend on it. An error that
// is not one of this package's reads as it always does.
func English(err error) string {
	if err == nil {
		return ""
	}
	switch x := err.(type) {
	case inLang:
		return x.in(EN)
	case interface{ Unwrap() []error }:
		// errors.Join: the texts of its errors, one to a line.
		if fmt.Sprintf("%T", err) == "*errors.joinError" {
			var lines []string
			for _, e := range x.Unwrap() {
				lines = append(lines, English(e))
			}
			return strings.Join(lines, "\n")
		}
	}
	return err.Error()
}
