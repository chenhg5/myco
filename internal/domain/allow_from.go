package domain

import "slices"

// AllowFromEdit is either a replacement or an atomic edit of named senders.
type AllowFromEdit struct {
	Replace, Add, Remove *[]string
}

type AllowFromChange struct {
	Added, Removed, Unchanged []string
}

func (e AllowFromEdit) Present() bool { return e.Replace != nil || e.Add != nil || e.Remove != nil }

// Apply keeps unrestricted access explicit: removing the final named sender
// must never turn a restricted list into the legacy empty (anyone) list.
func (e AllowFromEdit) Apply(a *Agent, acceptExecRisk bool) (*AllowFromChange, error) {
	if e.Replace != nil && (e.Add != nil || e.Remove != nil) {
		return nil, Errf(CodeInvalidArgument, "allow_from cannot be combined with allow_add or allow_remove")
	}
	parse := func(p *[]string) ([]AgentName, error) {
		if p == nil {
			return nil, nil
		}
		return ParseAgentNames(*p)
	}
	add, err := parse(e.Add)
	if err != nil {
		return nil, err
	}
	remove, err := parse(e.Remove)
	if err != nil {
		return nil, err
	}
	for _, n := range add {
		if slices.Contains(remove, n) {
			return nil, Errf(CodeInvalidArgument, "%s occurs in both allow_add and allow_remove", n)
		}
	}
	before := slices.Clone(a.AllowFrom)
	if e.Replace != nil {
		names, err := parse(e.Replace)
		if err != nil {
			return nil, err
		}
		if err := a.SetAllowFrom(names, acceptExecRisk); err != nil {
			return nil, err
		}
	} else if len(before) == 0 || slices.Contains(before, AnySender) {
		if len(remove) != 0 {
			return nil, Errf(CodeInvalidArgument, "cannot remove individual senders while anyone is allowed").WithSuggestion("explicitly replace allow_from with the permitted senders")
		}
		// Everyone is already allowed; adding a name must not restrict others.
	} else {
		next := make([]AgentName, 0, len(before)+len(add))
		for _, n := range before {
			if !slices.Contains(remove, n) {
				next = append(next, n)
			}
		}
		for _, n := range add {
			if !slices.Contains(next, n) {
				next = append(next, n)
			}
		}
		if len(next) == 0 {
			return nil, Errf(CodeInvalidArgument, "removing the final sender would create an unrestricted list").WithSuggestion("use the App's nobody selection to deny all senders; use an explicit replacement to allow anyone")
		}
		if err := a.SetAllowFrom(next, false); err != nil {
			return nil, err
		}
	}
	return DiffAllowFrom(before, a.AllowFrom), nil
}

// DiffAllowFrom compares actual stored lists, preserving their display order.
func DiffAllowFrom(before, after []AgentName) *AllowFromChange {
	d := &AllowFromChange{Added: []string{}, Removed: []string{}, Unchanged: []string{}}
	for _, n := range before {
		if slices.Contains(after, n) {
			d.Unchanged = append(d.Unchanged, string(n))
		} else {
			d.Removed = append(d.Removed, string(n))
		}
	}
	for _, n := range after {
		if !slices.Contains(before, n) {
			d.Added = append(d.Added, string(n))
		}
	}
	return d
}
