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
		fmt.Printf(format, args...)
	}
}

// Println is a drop-in for fmt.Println that no-ops in structured mode.
func Println(args ...any) {
	if current == FormatText {
		fmt.Println(args...)
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
