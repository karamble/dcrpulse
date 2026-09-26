package gamingfunds

import "sort"

// PrunableGroups returns the group chats whose protocol history no fund still
// depends on. A group qualifies only when every accepted table in it was
// funded, every deposit this ledger holds for those tables is spent by a
// transaction spendDepth reports at least depth blocks deep, and no
// settlement, operation, preview or recovery quote for them is still open.
// Any spendDepth error qualifies nothing.
func (s *Store) PrunableGroups(now, depth int64, spendDepth func(txid string) (int64, error)) ([]string, error) {
	s.mu.Lock()
	d, err := s.load()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}

	byTable := make(map[string][]Deposit)
	for _, dep := range d.Deposits {
		key, err := scopeKey(dep.Scope, dep.Terms.Table)
		if err != nil {
			continue
		}
		byTable[key] = append(byTable[key], dep)
	}
	groups := make(map[string][]TableAuthorization)
	for _, t := range d.Tables {
		groups[t.Group] = append(groups[t.Group], t)
	}

	var out []string
	for group, tables := range groups {
		if group == "" {
			continue
		}
		deposits, open := groupDeposits(d, tables, byTable, now)
		if open {
			continue
		}
		settled := true
		for _, dep := range deposits {
			n, err := spendDepth(dep.SpendingTx)
			if err != nil {
				return nil, err
			}
			if n < depth {
				settled = false
				break
			}
		}
		if settled {
			out = append(out, group)
		}
	}
	sort.Strings(out)
	return out, nil
}

// groupDeposits collects the deposits of a group's tables and reports whether
// any of their money is still open in the ledger.
func groupDeposits(d diskState, tables []TableAuthorization, byTable map[string][]Deposit, now int64) ([]Deposit, bool) {
	var deposits []Deposit
	ids := make(map[string]bool)
	outpoints := make(map[string]bool)
	for _, t := range tables {
		key, err := scopeKey(t.Scope, t.Table)
		if err != nil || len(byTable[key]) == 0 {
			return nil, true
		}
		for _, dep := range byTable[key] {
			if dep.State != "spent" || dep.SpendingTx == "" {
				return nil, true
			}
			deposits = append(deposits, dep)
			ids[dep.ID] = true
			if prev, err := parseOutput(dep.Outpoint); err == nil {
				outpoints[prev.String()] = true
			}
		}
		for _, p := range d.Settlements {
			if p.Scope == t.Scope && p.Table == t.Table &&
				p.State != "confirmed" && p.State != "expired" && p.State != "rejected" {
				return nil, true
			}
		}
	}
	for _, op := range d.Operations {
		if op.State == "confirmed" {
			continue
		}
		for _, id := range op.DepositIDs {
			if ids[id] {
				return nil, true
			}
		}
		for _, input := range op.Inputs {
			if outpoints[input] {
				return nil, true
			}
		}
	}
	for _, p := range d.Previews {
		if ids[p.DepositID] && p.ExpiresAt > now {
			return nil, true
		}
	}
	for _, q := range d.Quotes {
		if ids[q.DepositID] && q.TxID == "" && q.ExpiresAt > now {
			return nil, true
		}
	}
	return deposits, false
}
