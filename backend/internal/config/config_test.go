package config

import "testing"

func TestRemoteBindingOptIn(t *testing.T) {
	t.Setenv("KUBESCALE_NAMESPACES", "demo")
	t.Setenv("KUBESCALE_LISTEN", "0.0.0.0:8080")
	t.Setenv("KUBESCALE_ALLOW_REMOTE", "false")
	t.Setenv("KUBESCALE_WEBHOOK_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("remote enabled by default")
	}
	t.Setenv("KUBESCALE_ALLOW_REMOTE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("remote without webhook token")
	}
	t.Setenv("KUBESCALE_WEBHOOK_TOKEN", "test")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBESCALE_CONTROLLER_INTERVAL", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("remote must validate durations")
	}
}
