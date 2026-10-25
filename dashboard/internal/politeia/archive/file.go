// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// FileFormat names an exported archive.
const FileFormat = "dcrpulse-politeia-archive"

// File is an exported archive. Its proposals are verified again on import;
// nothing in the file is trusted.
type File struct {
	Format        string      `json:"format"`
	SchemaVersion int         `json:"schemaVersion"`
	ServerKey     string      `json:"serverKey"`
	Proposals     []*Proposal `json:"proposals"`
}

// Export returns every archived proposal, ordered by token.
func (s *Store) Export() File {
	f := File{Format: FileFormat, SchemaVersion: SchemaVersion, ServerKey: s.serverKey}
	for _, e := range s.All() {
		f.Proposals = append(f.Proposals, e.Proposal)
	}
	sort.Slice(f.Proposals, func(i, j int) bool { return f.Proposals[i].Token() < f.Proposals[j].Token() })
	return f
}

// ImportResult is what an import stored, kept and refused.
type ImportResult struct {
	Imported int       `json:"imported"`
	Kept     int       `json:"kept"`
	Refused  []Refusal `json:"refused,omitempty"`
}

// Refusal is a proposal an import left out, and why.
type Refusal struct {
	Token  string `json:"token"`
	Reason string `json:"reason"`
}

// Import reads an exported archive one proposal at a time. Each is verified
// and stored unless the archive holds a copy fetched at the same time or
// later; one that fails is refused and the rest go on.
func (s *Store) Import(r io.Reader) (ImportResult, error) {
	var res ImportResult
	dec := json.NewDecoder(r)
	if err := expectDelim(dec, '{'); err != nil {
		return res, err
	}
	format := ""
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return res, err
		}
		switch t {
		case "format":
			if err := dec.Decode(&format); err != nil {
				return res, err
			}
			if format != FileFormat {
				return res, errors.New("not a dcrpulse Politeia archive")
			}
		case "proposals":
			if format == "" {
				return res, errors.New("not a dcrpulse Politeia archive")
			}
			if err := expectDelim(dec, '['); err != nil {
				return res, err
			}
			for dec.More() {
				var p Proposal
				if err := dec.Decode(&p); err != nil {
					return res, err
				}
				p.Source = SourceImport
				switch _, stored, err := s.put(&p, true); {
				case err != nil:
					res.Refused = append(res.Refused, Refusal{Token: p.Token(), Reason: err.Error()})
				case stored:
					res.Imported++
				default:
					res.Kept++
				}
			}
			if err := expectDelim(dec, ']'); err != nil {
				return res, err
			}
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return res, err
			}
		}
	}
	if format == "" {
		return res, errors.New("not a dcrpulse Politeia archive")
	}
	return res, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t != want {
		return fmt.Errorf("unexpected %v, want %v", t, want)
	}
	return nil
}
