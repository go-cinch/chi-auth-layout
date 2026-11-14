package app

import (
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestSliderCaptchaFromConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Captcha.Slider.TTL = time.Minute
	cfg.Auth.Captcha.Slider.MinimumDuration = time.Second
	if _, err := sliderCaptchaFromConfig(cfg, authmodule.NewMemoryPointCaptchaStore()); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.Captcha.Slider.TTL = 0
	if _, err := sliderCaptchaFromConfig(cfg, authmodule.NewMemoryPointCaptchaStore()); err == nil {
		t.Fatal("invalid slider captcha configuration was accepted")
	}
}
