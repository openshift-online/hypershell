package confirm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openshift-online/hypershell/components/cli/pkg/output"
)

// ErrCancelled is returned when the user declines a confirmation.
var ErrCancelled = errors.New("cancelled, nothing was changed")

// Ask asks the user to confirm a destructive action. It returns nil when the
// action may proceed: assumeYes is set, or the user answered y or yes at the
// prompt. It fails instead of guessing when stdin is not a terminal and
// assumeYes is not set, so scripts never delete by accident or by EOF.
func Ask(prompt string, assumeYes bool) error {
	return ask(prompt, assumeYes, os.Stdin, os.Stderr, output.IsTerminal(os.Stdin))
}

func ask(prompt string, assumeYes bool, in io.Reader, out io.Writer, interactive bool) error {
	if assumeYes {
		return nil
	}
	if !interactive {
		return fmt.Errorf("confirmation required but stdin is not a terminal, pass --yes to proceed")
	}
	fmt.Fprintf(out, "%s [y/N]: ", prompt)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && answer == "" {
		return ErrCancelled
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return ErrCancelled
}
