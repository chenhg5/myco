package app

import (
	"context"
	"github.com/chenhg5/myco/internal/domain"
)

type templateLocaleCache struct {
	blob    string
	locales map[string]domain.TemplateLocale
}

func (s *TemplateService) templateLocales(ctx context.Context, id, blob string) map[string]domain.TemplateLocale {
	if s.pages == nil || s.blobs == nil {
		return nil
	}
	if cached, ok := s.localeCache.Load(id); ok && cached.(templateLocaleCache).blob == blob {
		return cached.(templateLocaleCache).locales
	}
	z, close, err := s.pages.openZip(ctx, blob)
	if err != nil {
		return nil
	}
	defer close()
	raw, _, err := readZipFile(z, "locales.json", 32<<10)
	if err != nil {
		return nil
	}
	locales, _ := domain.ParseTemplateLocales(raw)
	s.localeCache.Store(id, templateLocaleCache{blob, locales})
	return locales
}
