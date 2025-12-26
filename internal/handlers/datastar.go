package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// DatastarSSE helper for sending Datastar-compatible SSE events
// Uses Fiber v3's native SendStreamWriter - no external SDK needed
type DatastarSSE struct {
	w *bufio.Writer
}

// NewDatastarSSE creates a new SSE helper
func NewDatastarSSE(w *bufio.Writer) *DatastarSSE {
	return &DatastarSSE{w: w}
}

// PatchSignals sends a datastar-patch-signals event to update client-side signals
// Signals are merged into the existing signal store using JSON Merge Patch semantics
func (d *DatastarSSE) PatchSignals(signals map[string]any) error {
	data, err := json.Marshal(signals)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "event: datastar-patch-signals\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: signals %s\n\n", data); err != nil {
		return err
	}
	return d.w.Flush()
}

// PatchElements sends a datastar-patch-elements event to update DOM elements
// selector: CSS selector for target element(s)
// html: HTML content to patch
func (d *DatastarSSE) PatchElements(selector, html string) error {
	if _, err := fmt.Fprintf(d.w, "event: datastar-patch-elements\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: selector %s\n", selector); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: elements %s\n\n", html); err != nil {
		return err
	}
	return d.w.Flush()
}

// PatchElementsWithMode sends elements with a specific patch mode
// mode: "inner" (default), "outer", "prepend", "append", "before", "after", "remove"
func (d *DatastarSSE) PatchElementsWithMode(selector, html, mode string) error {
	if _, err := fmt.Fprintf(d.w, "event: datastar-patch-elements\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: mode %s\n", mode); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: selector %s\n", selector); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.w, "data: elements %s\n\n", html); err != nil {
		return err
	}
	return d.w.Flush()
}

// ExecuteScript executes JavaScript by appending a script element to the body
// This is the Datastar-recommended way to execute scripts via SSE
func (d *DatastarSSE) ExecuteScript(script string) error {
	scriptElement := fmt.Sprintf(`<script type="text/javascript">%s</script>`, script)
	return d.PatchElementsWithMode("body", scriptElement, "append")
}
