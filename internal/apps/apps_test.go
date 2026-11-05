package apps_test

import (
	"testing"

	"github.com/vortechron/reverb-go/internal/apps"
)

func TestRegistryFindsByKeyAndID(t *testing.T) {
	reg, err := apps.NewRegistry([]apps.App{{ID: "1", Key: "k1", Secret: "s1"}, {ID: "2", Key: "k2", Secret: "s2"}})
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := reg.ByKey("k2"); !ok || a.ID != "2" {
		t.Fatalf("ByKey = %+v %v", a, ok)
	}
	if a, ok := reg.ByID("1"); !ok || a.Key != "k1" {
		t.Fatalf("ByID = %+v %v", a, ok)
	}
	if _, ok := reg.ByKey("nope"); ok {
		t.Fatal("unknown key must not be found")
	}
	if len(reg.All()) != 2 {
		t.Fatalf("All = %d", len(reg.All()))
	}
}

func TestRegistryRejectsBadApps(t *testing.T) {
	cases := map[string][]apps.App{
		"empty list":    nil,
		"missing id":    {{Key: "k", Secret: "s"}},
		"missing key":   {{ID: "1", Secret: "s"}},
		"missing sec":   {{ID: "1", Key: "k"}},
		"duplicate key": {{ID: "1", Key: "k", Secret: "s"}, {ID: "2", Key: "k", Secret: "s"}},
		"duplicate id":  {{ID: "1", Key: "a", Secret: "s"}, {ID: "1", Key: "b", Secret: "s"}},
	}
	for name, list := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := apps.NewRegistry(list); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		allowed []string
		origin  string
		want    bool
	}{
		{nil, "http://anything.test", true},
		{[]string{"*"}, "", true},
		{[]string{"app.test"}, "https://app.test:8443", true},
		{[]string{"app.test"}, "https://evil.test", false},
		{[]string{"*.app.test"}, "https://a.app.test", true},
		{[]string{"*.app.test"}, "https://app.test", false},
		{[]string{"app.test"}, "", false},
		{[]string{"app.test"}, "::bad", false},
	}
	for _, c := range cases {
		a := apps.App{AllowedOrigins: c.allowed}
		if got := a.OriginAllowed(c.origin); got != c.want {
			t.Errorf("OriginAllowed(%v, %q) = %v, want %v", c.allowed, c.origin, got, c.want)
		}
	}
}
