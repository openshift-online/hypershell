package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"time"
)

type Setting struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Value   string   `json:"value"`
	Effect  string   `json:"effect"`
	Kind    string   `json:"kind"`
	Choices []string `json:"choices"`
	Help    string   `json:"help"`
	Saved   *string  `json:"saved"`
}
type settingsResult struct {
	err     error
	message string
}
type settingsDone settingsResult
type settingsRequest struct {
	ctx        context.Context
	key, value string
	done       chan settingsResult
}

func (e *Engine) config() Config   { e.cfgMu.RLock(); defer e.cfgMu.RUnlock(); return e.c }
func (e *Engine) adapter() Adapter { e.cfgMu.RLock(); defer e.cfgMu.RUnlock(); return e.a }
func (e *Engine) requestSetting(ctx context.Context, key, value string) settingsResult {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request := settingsRequest{ctx, key, value, make(chan settingsResult, 1)}
	select {
	case e.settings <- request:
	case <-ctx.Done():
		return settingsResult{err: fmt.Errorf("dispatcher is busy; settings were not saved: %w", ctx.Err())}
	}
	select {
	case result := <-request.done:
		return result
	case <-ctx.Done():
		return settingsResult{err: fmt.Errorf("save could not be confirmed; inspect the settings file: %w", ctx.Err())}
	}
}
func (e *Engine) saveSetting(ctx context.Context, key, value string) settingsResult {
	if err := ctx.Err(); err != nil {
		return settingsResult{err: err}
	}
	c := e.config()
	var selected *Setting
	for i := range c.Settings {
		if c.Settings[i].Key == key {
			row := c.Settings[i]
			selected = &row
			break
		}
	}
	if selected == nil || selected.Kind == "readonly" {
		return settingsResult{err: fmt.Errorf("this setting is read only")}
	}
	var reply struct {
		Value    json.RawMessage `json:"value"`
		Settings []Setting       `json:"settings"`
	}
	if err := e.adapter().call(ctx, "settings-save", map[string]string{"key": key, "value": value}, &reply); err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("save unconfirmed; inspect %s: %w", c.SettingsFile, ctx.Err())
		}
		return settingsResult{err: err}
	}
	var parsed any
	decoder := json.NewDecoder(bytes.NewReader(reply.Value))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		return settingsResult{err: fmt.Errorf("saved, but cannot decode active value: %w", err)}
	}
	normalized := fmt.Sprint(parsed)
	message := "Saved to " + c.SettingsFile + ". " + selected.Effect + "."
	if selected.Effect != "Restart required" {
		values := map[string]any{}
		if c.Runtime != "" {
			if err := json.Unmarshal([]byte(c.Runtime), &values); err != nil {
				return settingsResult{err: fmt.Errorf("saved, but cannot apply runtime settings: %w", err)}
			}
		}
		values[key] = parsed
		encoded, err := json.Marshal(values)
		if err != nil {
			return settingsResult{err: err}
		}
		c.Runtime = string(encoded)
		n, _ := strconv.Atoi(normalized) // Numeric input has been validated by the preference writer.
		switch key {
		case "concurrency":
			c.Concurrency = n
		case "poll_interval":
			c.Poll = n
		case "auth_check_interval":
			c.AuthCheck = n
		case "auth_retry":
			c.AuthRetry = n
		case "backoff":
			c.Backoff = n
		case "backoff_max":
			c.BackoffMax = n
		case "auto_merge":
			env := make(map[string]*string, len(c.Environment)+1)
			for k, v := range c.Environment {
				env[k] = v
			}
			v := normalized
			env["FULLSEND_AUTO_MERGE"] = &v
			c.Environment = env
		case "model":
			c.Model = normalized
		case "ANTHROPIC_VERTEX_PROJECT_ID", "CLOUD_ML_REGION", "GOOGLE_APPLICATION_CREDENTIALS":
			env := map[string]*string{}
			for k, v := range c.Environment {
				env[k] = v
			}
			v := normalized
			env[key] = &v
			c.Environment = env
		}
		for i := range reply.Settings {
			if reply.Settings[i].Key == key {
				reply.Settings[i].Value = normalized
			}
		}
	}
	seen := map[string]bool{}
	for _, item := range reply.Settings {
		seen[item.Key] = true
	}
	for _, item := range c.Settings {
		if item.Kind == "readonly" && !seen[item.Key] {
			reply.Settings = append(reply.Settings, item)
		}
	}
	c.Settings = reply.Settings
	e.cfgMu.Lock()
	e.c = c
	e.a.Runtime = c.Runtime
	e.cfgMu.Unlock()
	e.note("Configuration saved: " + key + " (" + selected.Effect + ")")
	return settingsResult{message: message}
}

func (m Model) configurationView(lines []string, w, h int) string {
	c := m.engine.config()
	lines = append(lines, m.viewTabsFor(2, w)...)
	lines = append(lines, "")
	lines = append(lines, m.style("SETTINGS FILE", "8", true))
	lines = append(lines, m.wrapped(c.SettingsFile, w-2, "6")...)
	lines = append(lines, m.wrapped("Saving writes this file on disk. Manual file edits load on restart. Explicit launch arguments override saved values on restart.", w-2, "8")...)
	lines = append(lines, "")
	if m.state.Monitoring {
		lines = append(lines, m.wrapped("Monitoring another dispatcher. These are launch settings; edit in its owner terminal.", w-2, "3")...)
	}
	if len(c.Settings) == 0 {
		lines = append(lines, "Configuration is not available for this launcher.", "", m.style("esc back   q detach", "8", false))
		return m.frame(lines, w, h)
	}
	index := min(m.configIndex, len(c.Settings)-1)
	row := c.Settings[index]
	if m.configEditing {
		lines = append(lines, m.style(row.Label, "7", true), m.style(row.Effect, "3", false))
		lines = append(lines, m.wrapped("Active now: "+row.Value, w-2, "8")...)
		if row.Kind == "readonly" {
			lines = append(lines, m.wrapped(row.Value, w-2, "7")...)
			lines = append(lines, "", m.style(row.Help, "8", false), "", m.style("enter / esc back   q detach", "8", false))
			return m.frame(lines, w, h)
		}
		input := m.configInput
		if row.Kind == "choice" {
			for i, choice := range row.Choices {
				prefix := "  "
				if i == m.configChoice {
					prefix = "> "
				}
				line := prefix + choice
				if i == m.configChoice {
					line = m.selection(line, w-2)
				}
				lines = append(lines, line)
			}
		} else {
			cursor := min(m.configCursor, len([]rune(input)))
			r := []rune(input)
			input = string(r[:cursor]) + "▏" + string(r[cursor:])
			start := max(0, ansi.StringWidth(string(r[:cursor]))-max(1, w-5))
			lines = append(lines, m.style("> "+ansi.Cut(input, start, start+max(1, w-4)), "6", true))
		}
		lines = append(lines, "")
		lines = append(lines, m.wrapped(row.Help, w-2, "8")...)
		if m.configError != "" {
			lines = append(lines, m.wrapped(m.configError, w-2, "1")...)
		}
		footer := "enter save to disk   esc discard   ctrl+c detach"
		if m.configSaving {
			footer = "Saving to disk…"
		}
		lines = append(lines, "", m.style(footer, "8", false))
		return m.frame(lines, w, h)
	}
	content := []string{}
	positions := make([]int, len(c.Settings))
	previous := ""
	for i, item := range c.Settings {
		if item.Effect != previous {
			if len(content) > 0 {
				content = append(content, "")
			}
			content = append(content, m.groupHeading(item.Effect, "", w-2))
			previous = item.Effect
		}
		positions[i] = len(content)
		value := item.Value
		if value == "" {
			value = "Not set / default"
		}
		line := "  " + cell(item.Label, max(1, w/2-2)) + " " + middle(value, max(1, w/2-3))
		if i == index {
			line = m.selection("> "+line[2:], w-2)
		}
		content = append(content, line)
		if item.Saved != nil && *item.Saved != item.Value {
			content = append(content, m.style("  Saved: "+middle(*item.Saved, max(1, w-12))+" · inactive", "3", false))
		}
	}
	count := max(1, h-len(lines)-5)
	start := max(0, positions[index]-count+1)
	end := min(len(content), start+count)
	lines = append(lines, m.withScrollbar(content[start:end], start, end, len(content), count, w)...)
	detail := row.Help
	if row.Saved != nil && *row.Saved != row.Value {
		detail = "Saved: " + *row.Saved + " · Active: " + row.Value + ". Restart required."
		if row.Effect != "Restart required" {
			detail = "A launch argument overrides the saved value on restart."
		}
	}
	lines = append(lines, m.style(middle(detail, w), "3", false), m.scrollIndicator(start, end, len(content)), "", m.style("j/k select   enter edit   tab Agents   esc back   ? help   q detach", "8", false))
	return m.frame(lines, w, h)
}
