package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

type PrinterBuilder struct {
	writer io.Writer
	pager  string
}

type Printer struct {
	writer      io.Writer
	terminal    bool
	width       int
	height      int
	pagerCmd    *exec.Cmd
	pagerStop   chan int
	pagerReader *os.File
	pagerWriter *os.File
}

var _ io.Writer = (*Printer)(nil)

func NewPrinter() *PrinterBuilder {
	return &PrinterBuilder{
		writer: os.Stdout,
	}
}

func (b *PrinterBuilder) Writer(value io.Writer) *PrinterBuilder {
	b.writer = value
	return b
}

func (b *PrinterBuilder) Pager(value string) *PrinterBuilder {
	b.pager = value
	return b
}

func (b *PrinterBuilder) Build(ctx context.Context) (result *Printer, err error) {
	if b.writer == nil {
		err = fmt.Errorf("writer is mandatory")
		return
	}

	pagerEnabled, pagerPath, pagerArgs, err := b.pagerCommand()
	if err != nil {
		return
	}

	terminal, width, height, err := b.isTerminal(b.writer)
	if err != nil {
		return
	}

	writer := b.writer
	var pagerCmd *exec.Cmd
	var pagerStop chan int
	var pagerReader, pagerWriter *os.File
	if pagerEnabled && terminal {
		pagerReader, pagerWriter, err = os.Pipe()
		if err != nil {
			return
		}

		pagerCmd = exec.Command(pagerPath, pagerArgs...)
		pagerCmd.Stdin = pagerReader
		pagerCmd.Stdout = writer
		err = pagerCmd.Start()
		if err != nil {
			pagerReader.Close()
			pagerWriter.Close()
			return
		}

		pagerStop = make(chan int)
		go func() {
			pagerCmd.Wait()
			pagerReader.Close()
			pagerWriter.Close()
			close(pagerStop)
		}()
	}

	result = &Printer{
		writer:      writer,
		terminal:    terminal,
		width:       width,
		height:      height,
		pagerCmd:    pagerCmd,
		pagerStop:   pagerStop,
		pagerReader: pagerReader,
		pagerWriter: pagerWriter,
	}

	return
}

func (b *PrinterBuilder) isTerminal(writer io.Writer) (result bool, width, height int, err error) {
	file, ok := writer.(*os.File)
	if !ok {
		return
	}
	fd := int(file.Fd())
	result = term.IsTerminal(fd)
	if result {
		width, height, err = term.GetSize(fd)
		if err != nil {
			return
		}
	}
	return
}

func (b *PrinterBuilder) pagerCommand() (enabled bool, path string, args []string, err error) {
	if b.pager == "" {
		return
	}

	chunks := strings.Split(b.pager, " ")
	if len(chunks) == 0 {
		return
	}

	path, err = exec.LookPath(chunks[0])
	if errors.Is(err, exec.ErrNotFound) {
		err = nil
		return
	}

	enabled = true
	args = chunks[1:]

	return
}

func (p *Printer) Write(b []byte) (n int, err error) {
	writer := p.writer
	if p.pagerWriter != nil {
		writer = p.pagerWriter
	}
	n, err = writer.Write(b)
	return
}

func (p *Printer) Terminal() bool {
	return p.terminal
}

func (p *Printer) Width() int {
	return p.width
}

func (p *Printer) Height() int {
	return p.height
}

func (p *Printer) Close() error {
	if p.pagerCmd != nil {
		p.pagerReader.Close()
		p.pagerWriter.Close()
		<-p.pagerStop
	}
	return nil
}
