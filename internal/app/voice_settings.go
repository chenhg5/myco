package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

// What a person set for voice chats (the App's 设置 › 语音): the model the
// voice thinks with, the speech service it speaks with, and their keys.
// Kept per person in the hub's settings, so each of their devices (the
// desktop App, the phone) has it without setting it up again. The newest
// change wins: every device sends the time the person made the change.
//
// It holds API keys. They are handed only to the person's own devices
// (their console tokens), and never logged or audited.

// VoiceSettingsService keeps each viewer's voice settings.
type VoiceSettingsService struct {
	settings domain.SettingRepository
	mu       sync.Mutex
}

func NewVoiceSettingsService(settings domain.SettingRepository) *VoiceSettingsService {
	return &VoiceSettingsService{settings: settings}
}

func voiceKey(viewer string) string { return "voice:" + viewer }

// Limits of a voice settings document.
const (
	maxVoiceField     = 4096 // any one value
	maxVoiceProviders = 16   // speech services with a key
)

// Get answers the viewer's voice settings; a zero UpdatedAt when none
// were ever set.
func (s *VoiceSettingsService) Get(ctx context.Context, viewer string) (api.VoiceSettings, error) {
	var out api.VoiceSettings
	raw, err := s.settings.Get(ctx, voiceKey(viewer))
	if err != nil || raw == "" {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return api.VoiceSettings{}, nil
	}
	return out, nil
}

// Put keeps the settings unless the hub has newer ones (by UpdatedAt),
// and answers what it holds then.
func (s *VoiceSettingsService) Put(ctx context.Context, viewer string, in api.VoiceSettings) (api.VoiceSettings, error) {
	if viewer == "" {
		return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "no viewer")
	}
	if in.UpdatedAt.IsZero() {
		return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "updated_at is required: when the settings were changed")
	}
	// A clock ahead of the hub's must not win for ever.
	if now := time.Now(); in.UpdatedAt.After(now.Add(5 * time.Minute)) {
		in.UpdatedAt = now
	}
	if in.TTSRate < -50 || in.TTSRate > 100 {
		return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "tts_rate must be between -50 and 100")
	}
	if len(in.TTSKeys) > maxVoiceProviders {
		return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "at most %d speech services", maxVoiceProviders)
	}
	in.LLMBaseURL, in.LLMModel, in.LLMKey = strings.TrimSpace(in.LLMBaseURL), strings.TrimSpace(in.LLMModel), strings.TrimSpace(in.LLMKey)
	in.TTSProvider, in.TTSSpeaker = strings.TrimSpace(in.TTSProvider), strings.TrimSpace(in.TTSSpeaker)
	keys := map[string]string{}
	for provider, key := range in.TTSKeys {
		provider, key = strings.TrimSpace(provider), strings.TrimSpace(key)
		if provider == "" || key == "" {
			continue
		}
		if len(provider) > 64 || len(key) > maxVoiceField {
			return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "a speech service's name or key is too long")
		}
		keys[provider] = key
	}
	in.TTSKeys = keys
	if len(keys) == 0 {
		in.TTSKeys = nil
	}
	for _, v := range []string{in.LLMBaseURL, in.LLMModel, in.LLMKey, in.TTSProvider, in.TTSSpeaker} {
		if len(v) > maxVoiceField {
			return api.VoiceSettings{}, domain.Errf(domain.CodeInvalidArgument, "a value is too long (%d bytes at most)", maxVoiceField)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	have, err := s.Get(ctx, viewer)
	if err != nil {
		return api.VoiceSettings{}, err
	}
	if !have.UpdatedAt.IsZero() && have.UpdatedAt.After(in.UpdatedAt) {
		// Another device changed them later: that is the answer.
		return have, nil
	}
	in.UpdatedAt = in.UpdatedAt.UTC()
	raw, _ := json.Marshal(in)
	if err := s.settings.Set(ctx, voiceKey(viewer), string(raw)); err != nil {
		return api.VoiceSettings{}, err
	}
	return in, nil
}
