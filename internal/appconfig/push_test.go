package appconfig

import "testing"

func TestPushTestDeploymentFlag(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		args      []string
		want      bool
	}{
		{"default", "", nil, false}, {"environment", "true", nil, true}, {"disabled", "false", nil, false},
		{"cli enables", "false", []string{"--push-test-enabled"}, true}, {"cli disables", "true", []string{"--push-test-enabled=false"}, false},
		{"invalid env fails closed", "invalid", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WISP_PUSH_TEST_ENABLED", tc.env)
			cfg, err := Parse(tc.args)
			if err != nil || cfg.PushTestEnabled != tc.want {
				t.Fatalf("enabled=%t err=%v", cfg.PushTestEnabled, err)
			}
		})
	}
}
