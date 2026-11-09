package app

import (
	"context"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// SettingE2E turns on end-to-end encryption for the hub's messages.
const SettingE2E = "e2e"

// KeyHolder is one identity that can read a message: an agent, or a device
// of a member (an app, or a CLI that accepted an invite).
type KeyHolder struct {
	// Who: the agent's name, or "@console:<member>" for a member's device.
	Who string
	// Role: "recipient", "sender" or "owner" (the person behind either side).
	Role string
	Key  domain.PublicKey
	// Device names the member's device ("" for agents).
	Device string
}

// E2EService keeps public keys and says who may read a message; the hub
// never reads message bodies once they are sealed.
type E2EService struct {
	keys     domain.KeyRepository
	settings domain.SettingRepository
	agents   domain.AgentRepository
	console  *ConsoleService
	members  domain.MemberRepository
	clock    Clock
}

// NewE2EService wires the end-to-end encryption use cases.
func NewE2EService(keys domain.KeyRepository, settings domain.SettingRepository, agents domain.AgentRepository,
	console *ConsoleService, members domain.MemberRepository, clock Clock) *E2EService {
	return &E2EService{keys: keys, settings: settings, agents: agents, console: console, members: members, clock: clock}
}

// Enabled reports whether the hub's owner turned encryption on.
func (s *E2EService) Enabled(ctx context.Context) bool {
	v, err := s.settings.Get(ctx, SettingE2E)
	return err == nil && v == "on"
}

// SetEnabled turns encryption on or off (the owner decides; callers check).
func (s *E2EService) SetEnabled(ctx context.Context, on bool) error {
	v := "off"
	if on {
		v = "on"
	}
	return s.settings.Set(ctx, SettingE2E, v)
}

// PutKey records an identity's public keys (replacing older ones).
func (s *E2EService) PutKey(ctx context.Context, kind, id, enc, sign string) error {
	if enc == "" || sign == "" {
		return domain.Errf(domain.CodeInvalidArgument, "enc_key and sign_key are required")
	}
	return s.keys.Put(ctx, &domain.PublicKey{Kind: kind, ID: id, Enc: enc, Sign: sign, UpdatedAt: s.clock.Now()})
}

// KeysOf returns the keys of who: an agent's key, or every device key of the
// member behind "@console:<member>".
func (s *E2EService) KeysOf(ctx context.Context, who string) ([]KeyHolder, error) {
	if name, ok := strings.CutPrefix(who, ConsoleSenderPrefix); ok {
		m, err := s.members.GetByName(ctx, name)
		if err != nil {
			if domain.IsCode(err, domain.CodeNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return s.memberKeys(ctx, m.ID, who, "")
	}
	k, err := s.keys.Get(ctx, domain.KeyAgent, who)
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []KeyHolder{{Who: who, Key: *k}}, nil
}

func (s *E2EService) memberKeys(ctx context.Context, memberID, who, role string) ([]KeyHolder, error) {
	if memberID == "" {
		return nil, nil
	}
	if who == "" {
		m, err := s.members.Get(ctx, memberID)
		if err != nil {
			return nil, nil
		}
		who = ConsoleSenderPrefix + m.Name
	}
	devs, err := s.console.Devices(ctx, memberID)
	if err != nil {
		return nil, err
	}
	var out []KeyHolder
	for _, d := range devs {
		k, err := s.keys.Get(ctx, domain.KeyDevice, d.ID)
		if err != nil {
			continue
		}
		out = append(out, KeyHolder{Who: who, Role: role, Key: *k, Device: d.Name})
	}
	return out, nil
}

// Readers is who may read a message from sender to recipient: both sides
// and the people who own them. Missing lists the sides that have no key
// yet (then the message cannot be encrypted).
func (s *E2EService) Readers(ctx context.Context, sender, recipient string) (readers []KeyHolder, missing []string, err error) {
	add := func(hs []KeyHolder, role string) {
		for _, h := range hs {
			if h.Role == "" {
				h.Role = role
			}
			readers = append(readers, h)
		}
	}
	for _, side := range []struct{ who, role string }{{recipient, "recipient"}, {sender, "sender"}} {
		hs, err := s.KeysOf(ctx, side.who)
		if err != nil {
			return nil, nil, err
		}
		if len(hs) == 0 {
			missing = append(missing, side.who)
		}
		add(hs, side.role)
		// The person behind an agent reads what it sends and receives.
		if !IsConsoleSender(domain.AgentName(side.who)) {
			if a, err := s.agents.Get(ctx, domain.AgentName(side.who)); err == nil && a.Owner != "" {
				hs, err := s.memberKeys(ctx, a.Owner, "", "owner")
				if err != nil {
					return nil, nil, err
				}
				add(hs, "owner")
			}
		}
	}
	return readers, missing, nil
}
