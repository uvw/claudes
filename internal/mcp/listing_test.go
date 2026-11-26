package mcp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

type names []string

func TestNewListingNamesTheProfilesThatKeepEachServerOn(t *testing.T) {
	servers := []Server{{Name: "browser", State: On}, {Name: "docs", State: Off}}
	profiles := map[string]names{"web": {"browser"}, "all": {"browser", "docs"}}

	list := NewListing("/work", "", servers, profiles)
	if list.Dir != "/work" || len(list.Servers) != 2 || list.Servers[0].ProfileState != "" {
		t.Fatalf("listing %+v", list)
	}
	if !slices.Equal(list.Servers[0].Profiles, []string{"all", "web"}) || !slices.Equal(list.Servers[1].Profiles, []string{"all"}) {
		t.Errorf("profiles %q and %q", list.Servers[0].Profiles, list.Servers[1].Profiles)
	}
	list = NewListing("/work", "web", servers, profiles)
	if list.Servers[0].ProfileState != On || list.Servers[1].ProfileState != Off {
		t.Errorf("under web: %+v", list.Servers)
	}
	if list = NewListing("/work", "", nil, profiles); list.Servers == nil {
		t.Error("no servers encode as null")
	}
}

func TestProfilesListTheServersNothingKnows(t *testing.T) {
	got := Profiles(map[string]names{"b": {"x", "gone"}, "a": nil}, map[string]bool{"x": true})
	if len(got) != 2 || got[0].Name != "a" || got[0].Servers == nil || len(got[0].Unknown) != 0 ||
		!slices.Equal(got[1].Unknown, []string{"gone"}) {
		t.Fatalf("profiles %+v", got)
	}
}

func TestCheckProfileNamesTheProfilesThereAre(t *testing.T) {
	if err := CheckProfile(map[string]names{}, ""); err != nil {
		t.Errorf("no profile asked: %v", err)
	}
	if err := CheckProfile(map[string]names{"web": nil}, "web"); err != nil {
		t.Errorf("a profile there: %v", err)
	}
	if err := CheckProfile(map[string]names{}, "web"); err == nil || err.Error() != `no MCP profile "web": config.yaml has no mcp.profiles yet` {
		t.Errorf("no profiles: %v", err)
	}
	if err := CheckProfile(map[string]names{"b": nil, "a": nil}, "web"); err == nil || err.Error() != `no MCP profile "web". Profiles: a, b` {
		t.Errorf("other profiles: %v", err)
	}
}

func TestAListingNamesItsAccountOnlyWhenSet(t *testing.T) {
	l := NewListing("/d", "", nil, map[string]names(nil))
	data, err := json.Marshal(l)
	if err != nil || strings.Contains(string(data), "account") {
		t.Fatalf("json %s, err %v", data, err)
	}
	if s := StoreIn("/p", "/p/.claude.json"); s.Home != "/p" || s.ConfigFile != "/p/.claude.json" {
		t.Fatalf("store %+v", s)
	}
}
