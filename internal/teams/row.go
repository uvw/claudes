package teams

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

// Row is a team as claudes team ls shows it.
type Row struct {
	Team      string   `json:"team"`
	LeadID    string   `json:"id"`
	SessionID string   `json:"sessionId"`
	Lead      string   `json:"lead"`
	Cwd       string   `json:"cwd"`
	Running   bool     `json:"running"`
	Teammates []string `json:"teammates"`
	Tasks     []Task   `json:"tasks"`
	// Account names the account whose directory holds the team, when claudes
	// manages more than one.
	Account string `json:"account,omitempty"`
}

// RowOf is team t joined with lead, the session that leads it. known is
// whether claude agents lists that session.
func RowOf(t Team, lead claude.Session, known bool) Row {
	r := Row{
		Team: t.Name, LeadID: cmp.Or(lead.JobID, claude.ShortID(t.LeadSessionID)), SessionID: t.LeadSessionID,
		Lead: lead.Name, Cwd: lead.Cwd, Running: known && lead.Running(), Teammates: t.Teammates(),
	}
	if r.Cwd == "" && len(t.Members) > 0 {
		r.Cwd = t.Members[0].Cwd
	}
	return r
}

func (r Row) Label() string { return cmp.Or(shell.Printable(r.Lead), r.LeadID) }

func (r Row) ID() string { return r.LeadID }

func (r Row) Aliases() []string { return claude.NonEmpty(r.Lead, r.LeadID, r.SessionID, r.Team) }

// TaskSummary is how many of a team's tasks are done, and who works on the
// open ones.
func TaskSummary(tasks []Task) string {
	if len(tasks) == 0 {
		return "-"
	}
	done := 0
	var working []string
	for _, t := range tasks {
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			working = append(working, shell.Printable(cmp.Or(t.Owner, "#"+t.ID)))
		}
	}
	s := fmt.Sprintf("%d/%d done", done, len(tasks))
	if len(working) > 0 {
		s += ", working: " + strings.Join(working, ", ")
	}
	return s
}
