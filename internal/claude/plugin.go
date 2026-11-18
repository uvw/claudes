package claude

import (
	"context"
	"encoding/json"
	"fmt"
)

// Plugin is one entry of `claude plugin list --json`. The entry also carries
// the plugin's MCP servers, with their URLs and commands, which claudes leaves
// out because they can hold tokens.
type Plugin struct {
	// ID is PLUGIN@MARKETPLACE.
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
}

// Marketplace is one entry of `claude plugin marketplace list --json`.
type Marketplace struct {
	Name string `json:"name"`
	// Source is github, git, url, directory, or another kind Claude Code
	// names, and Repo, URL, or Path says where.
	Source string `json:"source"`
	Repo   string `json:"repo"`
	URL    string `json:"url"`
	Path   string `json:"path"`
}

// Where is the marketplace's source as `claude plugin marketplace add` takes
// it: a GitHub repository, a URL, or a path.
func (m Marketplace) Where() string {
	switch {
	case m.Repo != "":
		return m.Repo
	case m.URL != "":
		return m.URL
	}
	return m.Path
}

// Plugins implements Agents.
func (c *Client) Plugins(ctx context.Context) ([]Plugin, error) {
	out, _, err := c.run(ctx, c.ListTimeout, "claude plugin list --json", "", "plugin", "list", "--json")
	if err != nil {
		return nil, err
	}
	var plugins []Plugin
	if err := json.Unmarshal([]byte(out), &plugins); err != nil {
		return nil, fmt.Errorf("claude plugin list --json returned something other than a list: %w", err)
	}
	return plugins, nil
}

// Marketplaces implements Agents.
func (c *Client) Marketplaces(ctx context.Context) ([]Marketplace, error) {
	out, _, err := c.run(ctx, c.ListTimeout, "claude plugin marketplace list --json", "", "plugin", "marketplace", "list", "--json")
	if err != nil {
		return nil, err
	}
	var list []Marketplace
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, fmt.Errorf("claude plugin marketplace list --json returned something other than a list: %w", err)
	}
	return list, nil
}

// AddMarketplace implements Agents.
func (c *Client) AddMarketplace(ctx context.Context, source string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude plugin marketplace add", "", "plugin", "marketplace", "add", source)
	return err
}

// RemoveMarketplace implements Agents.
func (c *Client) RemoveMarketplace(ctx context.Context, name string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude plugin marketplace remove "+name, "", "plugin", "marketplace", "remove", name)
	return err
}

// InstallPlugin implements Agents.
func (c *Client) InstallPlugin(ctx context.Context, id string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude plugin install "+id, "", "plugin", "install", id)
	return err
}

// UninstallPlugin implements Agents.
func (c *Client) UninstallPlugin(ctx context.Context, id string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude plugin uninstall "+id, "", "plugin", "uninstall", id)
	return err
}
