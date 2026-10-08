module github.com/openshift-online/hypershell/components/cli

go 1.26.4

toolchain go1.26.7

require (
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/nwidger/jsoncolor v0.3.2
	github.com/openshift-online/rh-trex-ai/components/api-server v0.0.0-20260925121109-368daa9d29e6
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.10
	gitlab.com/c0b/go-ordered-json v0.0.0-20201030195603-febf46534d5a
	golang.org/x/term v0.28.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/bubbles v1.0.0 // indirect
	github.com/charmbracelet/bubbletea v1.3.10 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/lipgloss v1.1.1-0.20250404203927-76690c660834 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/openshift-online/rh-trex-ai/components/api-server => github.com/rh-amarin/rh-trex-ai/components/api-server v0.0.0-20261007144355-9dcb81d79b36
