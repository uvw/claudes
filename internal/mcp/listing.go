package mcp

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Listing is what claudes mcp ls prints with -o json or yaml.
type Listing struct {
	Dir string `json:"dir"`
	// Profile is --profile, or "".
	Profile string `json:"profile"`
	Servers []Row  `json:"servers"`
}

// Row is one server in a listing.
type Row struct {
	Server
	// Profiles are the config.yaml profiles that keep the server on.
	Profiles []string `json:"profiles"`
	// ProfileState is the server's state under --profile, or "".
	ProfileState string `json:"profileState"`
}

// NewListing is the servers of dir, each with the profiles that keep it on.
// profiles maps each profile's name to the servers it keeps on. With profile,
// each row also has its state under that profile.
func NewListing[S ~[]string](dir, profile string, servers []Server, profiles map[string]S) Listing {
	list := Listing{Dir: dir, Profile: profile, Servers: make([]Row, 0, len(servers))}
	for _, s := range servers {
		row := Row{Server: s, Profiles: []string{}}
		for _, name := range slices.Sorted(maps.Keys(profiles)) {
			if slices.Contains(profiles[name], s.Name) {
				row.Profiles = append(row.Profiles, name)
			}
		}
		if profile != "" {
			row.ProfileState = Off
			if slices.Contains(profiles[profile], s.Name) {
				row.ProfileState = On
			}
		}
		list.Servers = append(list.Servers, row)
	}
	return list
}

// Profile is one profile, as claudes mcp profiles prints it.
type Profile struct {
	Name    string   `json:"name"`
	Servers []string `json:"servers"`
	// Unknown are the servers that match none in known.
	Unknown []string `json:"unknown"`
}

// Profiles are profiles in name order, each with the servers it names that
// are not in known.
func Profiles[S ~[]string](profiles map[string]S, known map[string]bool) []Profile {
	list := make([]Profile, 0, len(profiles))
	for _, name := range slices.Sorted(maps.Keys(profiles)) {
		p := Profile{Name: name, Servers: append([]string{}, profiles[name]...), Unknown: []string{}}
		for _, s := range p.Servers {
			if !known[s] {
				p.Unknown = append(p.Unknown, s)
			}
		}
		list = append(list, p)
	}
	return list
}

// CheckProfile refuses a profile name that profiles does not hold. An empty
// name asks for no profile, and passes.
func CheckProfile[S ~[]string](profiles map[string]S, name string) error {
	if _, ok := profiles[name]; name == "" || ok {
		return nil
	}
	have := slices.Sorted(maps.Keys(profiles))
	if len(have) == 0 {
		return fmt.Errorf("no MCP profile %q: config.yaml has no mcp.profiles yet", name)
	}
	return fmt.Errorf("no MCP profile %q. Profiles: %s", name, strings.Join(have, ", "))
}
