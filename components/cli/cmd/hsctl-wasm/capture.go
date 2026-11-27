//go:build js && wasm

package main

import (
	"bytes"
	"errors"
	"syscall/js"
)

// capture collects what a command writes to standard output and standard error,
// and gives it empty standard input, for the length of one call.
//
// Under js/wasm, Go writes file descriptors 1 and 2 and reads descriptor 0
// through the JavaScript object globalThis.fs: fs.write(fd, buffer, offset,
// length, position, callback) and fs.read with the same arguments. Many hsctl
// commands write to os.Stdout and os.Stderr directly, so capture replaces those
// two functions during the call and restores them afterwards. Other descriptors
// go to the original functions. This works with the default fs of wasm_exec.js
// in a browser worker and with the Node.js fs module of the Go js/wasm test
// runner.
type capture struct {
	fs        js.Value
	origWrite js.Value
	origRead  js.Value
	write     js.Func
	read      js.Func
	stdout    bytes.Buffer
	stderr    bytes.Buffer
}

func startCapture() (*capture, error) {
	fs := js.Global().Get("fs")
	if fs.Type() != js.TypeObject {
		return nil, errors.New("globalThis.fs is not set; load wasm_exec.js before the module")
	}
	c := &capture{fs: fs, origWrite: fs.Get("write"), origRead: fs.Get("read")}
	if c.origWrite.Type() != js.TypeFunction || c.origRead.Type() != js.TypeFunction {
		return nil, errors.New("globalThis.fs has no write or read function")
	}

	c.write = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 6 && args[5].Type() == js.TypeFunction {
			if fd := args[0].Int(); fd == 1 || fd == 2 {
				offset, length := args[2].Int(), args[3].Int()
				data := make([]byte, length)
				js.CopyBytesToGo(data, args[1].Call("subarray", offset, offset+length))
				if fd == 1 {
					c.stdout.Write(data)
				} else {
					c.stderr.Write(data)
				}
				args[5].Invoke(js.Null(), length)
				return nil
			}
		}
		return c.origWrite.Call("apply", this, toArray(args))
	})
	c.read = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 6 && args[5].Type() == js.TypeFunction && args[0].Int() == 0 {
			// End of file: standard input is empty.
			args[5].Invoke(js.Null(), 0)
			return nil
		}
		return c.origRead.Call("apply", this, toArray(args))
	})
	fs.Set("write", c.write)
	fs.Set("read", c.read)
	return c, nil
}

// stop restores the original functions and returns the captured output.
func (c *capture) stop() (stdout, stderr string) {
	c.fs.Set("write", c.origWrite)
	c.fs.Set("read", c.origRead)
	c.write.Release()
	c.read.Release()
	return c.stdout.String(), c.stderr.String()
}

func toArray(args []js.Value) js.Value {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg
	}
	return js.ValueOf(values)
}
