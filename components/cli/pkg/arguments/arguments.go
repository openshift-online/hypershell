package arguments

import (
	"strings"

	"github.com/spf13/pflag"
)

func AddParameterFlag(fs *pflag.FlagSet, values *[]string) {
	fs.StringArrayVarP(
		values,
		"parameter",
		"p",
		nil,
		"Query parameters to add to the request. Format: name=value. "+
			"Can be used multiple times.",
	)
}

func AddColumnsFlag(fs *pflag.FlagSet, value *string, defaultColumns string) {
	fs.StringVar(
		value,
		"columns",
		defaultColumns,
		"Comma-separated list of columns to display.",
	)
}

func AddNoHeadersFlag(fs *pflag.FlagSet, value *bool) {
	fs.BoolVar(
		value,
		"no-headers",
		false,
		"Don't print header row.",
	)
}

func AddOutputFlag(fs *pflag.FlagSet, value *string) {
	fs.StringVarP(
		value,
		"output",
		"o",
		"table",
		"Output format: table or json.",
	)
}

func ParseNameValuePair(text string) (name, value string) {
	position := strings.Index(text, "=")
	if position != -1 {
		name = strings.TrimSpace(text[:position])
		value = text[position+1:]
	} else {
		name = strings.TrimSpace(text)
		value = ""
	}
	return
}
