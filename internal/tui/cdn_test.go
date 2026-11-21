package tui

import "testing"

func TestIsCDNHost(t *testing.T) {
	cdn := []string{
		"autopatchhk.yuanshen.com",
		"autopatchcn.yuanshen.com",
		"autopatchcn.mihoyo.com",
		"a1881.w27.akamai.net",
	}
	for _, h := range cdn {
		if !isCDNHost(h) {
			t.Errorf("%s should be treated as CDN (direct)", h)
		}
	}
	api := []string{
		"sg-hyp-api.hoyoverse.com",
		"hyp-api.mihoyo.com",
		"sdk-static.mihoyo.com",
		"github.com",
	}
	for _, h := range api {
		if isCDNHost(h) {
			t.Errorf("%s should NOT be treated as CDN (use proxy)", h)
		}
	}
}
