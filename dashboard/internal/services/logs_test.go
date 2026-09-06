// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bufio"
	"strings"
	"testing"
)

// A bufio.Scanner stops at the first token larger than its buffer, so one
// over-long line used to hide every line after it — exactly when the operator
// most wants to see what followed. The reader must take the hit on that line
// alone and carry on.
func TestReadLogLineSurvivesAnOverLongLine(t *testing.T) {
	huge := strings.Repeat("A", 2*1024*1024)
	r := bufio.NewReaderSize(strings.NewReader("before\n"+huge+"\nafter\n"), 64*1024)

	first, err := readLogLine(r)
	if err != nil || first != "before" {
		t.Fatalf("first line = %q, err %v; want \"before\"", first, err)
	}

	long, err := readLogLine(r)
	if err != nil {
		t.Fatalf("reading the over-long line: %v", err)
	}
	if len(long) > tailLineMax+64 {
		t.Errorf("the over-long line came back at %d bytes; it must be bounded", len(long))
	}
	if !strings.Contains(long, "truncated") {
		t.Errorf("a clipped line is not marked as clipped: %q", long[:min(len(long), 80)])
	}

	// The assertion that matters: the line after the huge one is still reachable.
	last, _ := readLogLine(r)
	if last != "after" {
		t.Fatalf("the line after the over-long one was lost: %q", last)
	}
}

func TestReadLogLineBasics(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{name: "plain line", in: "hello\n", want: "hello"},
		{name: "carriage return stripped", in: "hello\r\n", want: "hello"},
		{name: "empty line", in: "\n", want: ""},
		{name: "unterminated final line", in: "tail", want: "tail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := readLogLine(bufio.NewReaderSize(strings.NewReader(tc.in), 64))
			if got != tc.want {
				t.Errorf("readLogLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
