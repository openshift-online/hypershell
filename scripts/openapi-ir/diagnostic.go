package ir

import "fmt"

type Diagnostic struct {
	Source  SourceLocation
	Context string
	Message string
}

func (diagnostic *Diagnostic) Error() string {
	location := diagnostic.Source.File
	if diagnostic.Source.Pointer != "" {
		location += "#" + diagnostic.Source.Pointer
	}
	if diagnostic.Context != "" {
		return fmt.Sprintf("%s: %s: %s", location, diagnostic.Context, diagnostic.Message)
	}
	return fmt.Sprintf("%s: %s", location, diagnostic.Message)
}

func newDiagnostic(source SourceLocation, context, format string, arguments ...any) error {
	return &Diagnostic{Source: source, Context: context, Message: fmt.Sprintf(format, arguments...)}
}
