//go:build windows

package island

import (
	"jbnplayer/internal/settings"
)

type appearanceResult struct {
	settings.Appearance
	Surfaces []string `json:"surfaces"`
	Error    string   `json:"error"`
}

func appearanceView(a settings.Appearance, err error) appearanceResult {
	r := appearanceResult{Appearance: a, Surfaces: settings.Surfaces}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

// GetAppearance returns the saved accent color and surface tone.
func (s *service) GetAppearance() appearanceResult {
	a, err := settings.LoadAppearance(s.appearancePath)
	return appearanceView(a, err)
}

// SetAppearance validates and saves the accent color and surface tone.
func (s *service) SetAppearance(a settings.Appearance) appearanceResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.Normalize()
	return appearanceView(a, settings.SaveAppearance(s.appearancePath, a))
}
