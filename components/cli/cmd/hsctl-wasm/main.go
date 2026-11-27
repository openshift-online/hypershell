//go:build js && wasm

// Command hsctl-wasm is hsctl for the web console terminal: the hsctl command
// tree built for GOOS=js GOARCH=wasm and run in a dedicated Web Worker
// (specs/platform/hsctl-browser-build.spec.md). It defines one global function,
// hsctlRun(argv), and sends API requests to the page origin with the console
// session cookie.
package main

import (
	"net/http"
	"syscall/js"
)

func main() {
	// http.DefaultTransport uses the browser Fetch API under js/wasm.
	js.Global().Set("hsctlRun", newBridge(http.DefaultTransport).jsFunc())

	// The worker calls hsctlRun after main returns control to the event loop;
	// keep the Go program alive to serve those calls.
	select {}
}
