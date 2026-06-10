package output

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

var current Format = FormatText

// sink, when set, receives the same text chunks written to stdout by Printf and
// Println. The desktop app uses it to stream progress to the UI; it is nil for
// the CLI.
var sink func(string)

// SetSink installs (or clears, with nil) the progress output sink.
func SetSink(s func(string)) { sink = s }

func Set(f Format)       { current = f }
func Get() Format        { return current }
func IsStructured() bool { return current != FormatText }

func Validate(s string) (Format, error) {
	switch Format(s) {
	case FormatText, FormatJSON, FormatYAML:
		return Format(s), nil
	}
	return "", fmt.Errorf("invalid output format %q: must be text, json, or yaml", s)
}

// Printf is a drop-in for fmt.Printf that no-ops in structured mode.
func Printf(format string, args ...any) {
	if current == FormatText {
		s := fmt.Sprintf(format, args...)
		fmt.Print(s)
		if sink != nil {
			sink(s)
		}
	}
}

// Println is a drop-in for fmt.Println that no-ops in structured mode.
func Println(args ...any) {
	if current == FormatText {
		s := fmt.Sprintln(args...)
		fmt.Print(s)
		if sink != nil {
			sink(s)
		}
	}
}

// Emit serializes v as JSON or YAML and writes it to stdout.
func Emit(v any) error {
	var b []byte
	var err error
	switch current {
	case FormatJSON:
		b, err = json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", b)
	case FormatYAML:
		b, err = yaml.Marshal(v)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(b)
	}
	return err
}
