// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
)

var useColor bool

func initColor(disabled bool) {
	if disabled {
		useColor = false
		return
	}
	if fi, err := os.Stdout.Stat(); err == nil {
		useColor = fi.Mode()&os.ModeCharDevice != 0
	}
}

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(s string) string    { return paint("31", s) }
func green(s string) string  { return paint("32", s) }
func yellow(s string) string { return paint("33", s) }
func cyan(s string) string   { return paint("36", s) }
func bold(s string) string   { return paint("1", s) }
func dim(s string) string    { return paint("2", s) }

func section(title string) {
	fmt.Printf("\n%s\n", bold("== "+title+" =="))
}

// rec is one recorded tool outcome.
type rec struct {
	domain string
	name   string
	status string
	note   string
}

var results []rec

func colorStatus(status string) string {
	switch status {
	case "OK", "GATED", "SENT":
		return green(fmt.Sprintf("%-5s", status))
	case "SKIP", "DOWN", "ERR":
		return yellow(fmt.Sprintf("%-5s", status))
	case "FAIL":
		return red(fmt.Sprintf("%-5s", status))
	default:
		return cyan(fmt.Sprintf("%-5s", status))
	}
}

// add records and prints a tool outcome.
func add(sp spec, status, note string) {
	results = append(results, rec{sp.domain, sp.name, status, note})
	line := fmt.Sprintf("  %s %-28s %s", colorStatus(status), sp.name, dim(note))
	fmt.Println(line)
}

func summary() {
	section("Summary")
	counts := map[string]int{}
	var fails []rec
	for _, r := range results {
		counts[r.status]++
		if r.status == "FAIL" {
			fails = append(fails, r)
		}
	}
	order := []string{"OK", "GATED", "SENT", "SKIP", "DOWN", "ERR", "FAIL"}
	var parts string
	for _, s := range order {
		if counts[s] > 0 {
			parts += fmt.Sprintf("  %s=%d", s, counts[s])
		}
	}
	fmt.Printf("  %d tools exercised:%s\n", len(results), parts)
	if len(fails) > 0 {
		fmt.Printf("\n  %s\n", red(fmt.Sprintf("%d gate(s)/check(s) did not hold:", len(fails))))
		for _, f := range fails {
			fmt.Printf("    %s %s\n", red(f.name), f.note)
		}
	} else {
		fmt.Printf("  %s\n", green("all checks held"))
	}
}
