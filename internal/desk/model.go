package desk

import (
	"errors"
	"strings"
)

type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Pros        string `json:"pros,omitempty"`
	Cons        string `json:"cons,omitempty"`
}
type Draft struct {
	Kind                 string   `json:"kind"`
	Title                string   `json:"title"`
	Context              string   `json:"context"`
	Options              []Option `json:"options,omitempty"`
	RecommendedOption    string   `json:"recommended_option,omitempty"`
	RecommendationReason string   `json:"recommendation_reason,omitempty"`
	CreatedBy            string   `json:"created_by"`
	OwnerCoordinator     string   `json:"owner_coordinator"`
	OriginSession        string   `json:"origin_session,omitempty"`
	TaskRef              string   `json:"task_ref,omitempty"`
	BlockingScope        string   `json:"blocking_scope,omitempty"`
	Branch               string   `json:"branch,omitempty"`
	Commit               string   `json:"commit,omitempty"`
	RelatedDocuments     []string `json:"related_documents,omitempty"`
	Priority             string   `json:"priority,omitempty"`
}
type Reply struct {
	Revision int    `json:"revision"`
	OptionID string `json:"option_id,omitempty"`
	Text     string `json:"text,omitempty"`
	Ack      bool   `json:"ack"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
}
type Issue struct {
	Draft
	ProjectID string `json:"project_id"`
	ID        string `json:"issue_id"`
	Revision  int    `json:"revision"`
	Status    string `json:"status"`
	Deferred  bool   `json:"deferred"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Reply     *Reply `json:"reply,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}
type Event struct {
	Seq      int64  `json:"seq"`
	ID       string `json:"event_id"`
	IssueID  string `json:"issue_id"`
	Revision int    `json:"revision"`
	Kind     string `json:"kind"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
	Issue    Issue  `json:"issue"`
}
type History struct {
	Event
	Receipts []Receipt `json:"receipts"`
}
type Receipt struct {
	Consumer string `json:"consumer"`
	Holder   string `json:"holder"`
	At       string `json:"at"`
}
type Lease struct {
	Consumer  string `json:"consumer"`
	Holder    string `json:"holder"`
	ExpiresAt string `json:"expires_at"`
}

var ErrConflict = errors.New("revision or state conflict; reload the issue")
var ErrNotFound = errors.New("not found")
var ErrLease = errors.New("consumer lease missing, expired, or held by another coordinator")

func (d *Draft) Validate() error {
	if d.Kind != "decision" && d.Kind != "action" && d.Kind != "notice" {
		return errors.New("kind must be decision, action, or notice")
	}
	if strings.TrimSpace(d.Title) == "" || len(d.Title) > 300 {
		return errors.New("title is required and must be at most 300 bytes")
	}
	if len(d.Context) > 200000 || len(d.Options) > 30 {
		return errors.New("context or options too large")
	}
	if d.Kind != "decision" && (len(d.Options) > 0 || d.RecommendedOption != "") {
		return errors.New("only decisions have options")
	}
	seen := map[string]bool{}
	for _, o := range d.Options {
		if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Label) == "" || seen[o.ID] {
			return errors.New("option IDs must be unique and nonempty; labels are required")
		}
		seen[o.ID] = true
	}
	if d.RecommendedOption != "" && !seen[d.RecommendedOption] {
		return errors.New("recommended_option must reference an option ID")
	}
	if d.CreatedBy == "" {
		d.CreatedBy = "agent"
	}
	if d.OwnerCoordinator == "" {
		d.OwnerCoordinator = "coordinator"
	}
	if d.Priority == "" {
		d.Priority = "normal"
	}
	if d.Priority != "normal" && d.Priority != "high" && d.Priority != "low" {
		return errors.New("priority must be low, normal, or high")
	}
	return nil
}
