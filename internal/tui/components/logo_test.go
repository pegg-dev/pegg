package components

import "testing"

func TestLogoGeometry(t *testing.T) {
	lg := DefaultLogo()
	if lg.Width != 24 {
		t.Errorf("DefaultLogo width = %d, want 24", lg.Width)
	}
	if lg.Height != 6 {
		t.Errorf("DefaultLogo height = %d, want 6", lg.Height)
	}
}

func TestLogoBlinkToggles(t *testing.T) {
	lg := DefaultLogo()
	lg.SetBlink(false)
	lg.OnTick(true)
	if !lg.blinkOn {
		t.Error("OnTick(true) should enable blink")
	}
}
