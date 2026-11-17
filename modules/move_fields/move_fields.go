// Package move_fields finds the fields a Jira transition requires and fills
// them before moving, automatically for every board (no admin config):
//
//   - team-managed projects keep required fields as workflow transition
//     validators ("system:validate-field-value", read with the operator
//     credential — the workflows endpoint may need admin rights);
//   - company-managed projects mark them required:true on the transition
//     screen (transitions?expand=transitions.fields);
//   - anything else Jira rejects a move over (a new rule kind, a stale
//     cache) comes back as a 400 that is turned into the same answer.
package move_fields

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/jira_client/jira_fields"
)

// CacheTTL: how long a (project, issue type) workflow's rules are reused.
const CacheTTL = 5 * time.Minute

// Transition is the move being made.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   string `json:"to"`
	// ToID is the target status id (matches a workflow's toStatusReference).
	ToID string `json:"-"`
}

// Required is what a move still needs: the 422 "fields_required" body.
type Required struct {
	Transition Transition          `json:"transition"`
	Message    string              `json:"message"`
	Fields     []jira_fields.Field `json:"fields"`
	IssueKey   string              `json:"issueKey"`
	IssueURL   string              `json:"issueUrl,omitempty"`
}

// FieldsRequiredError: the move was not made, Required says what to fill.
type FieldsRequiredError struct{ Required Required }

func (e *FieldsRequiredError) Error() string {
	names := make([]string, 0, len(e.Required.Fields))
	for _, f := range e.Required.Fields {
		names = append(names, f.Name)
	}
	return fmt.Sprintf("move_fields: %s needs %s before %q", e.Required.IssueKey, strings.Join(names, ", "), e.Required.Transition.To)
}

// FieldNames is "A", "A dan B", "A, B dan C".
func (e *FieldsRequiredError) FieldNames() string {
	names := make([]string, 0, len(e.Required.Fields))
	for _, f := range e.Required.Fields {
		names = append(names, f.Name)
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " dan " + names[len(names)-1]
}

// ValidationError: a submitted value can't be sent (per field id).
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	parts := []string{}
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "move_fields: invalid values: " + strings.Join(parts, "; ")
}

type rule struct {
	ids     []string
	message string
}

type cachedRules struct {
	at   time.Time
	byID map[string]rule
	byTo map[string]rule
}

type Service struct {
	jira  jira_client_service.TransitionFieldsClient
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]cachedRules // projectID|issueTypeID
}

func New(jira jira_client_service.TransitionFieldsClient) *Service {
	return &Service{jira: jira, now: time.Now, cache: map[string]cachedRules{}}
}

func cacheKey(projectID, typeID string) string { return projectID + "|" + typeID }

// rules returns the workflow validators' required fields per transition. A
// failing lookup (403 without admin rights, Jira down) is cached as "no
// rules" for CacheTTL: screen fields and the 400 fallback still apply.
func (s *Service) rules(ctx context.Context, projectID, typeID string) cachedRules {
	key := cacheKey(projectID, typeID)
	s.mu.Lock()
	c, ok := s.cache[key]
	s.mu.Unlock()
	if ok && s.now().Sub(c.at) < CacheTTL {
		return c
	}
	c = cachedRules{at: s.now(), byID: map[string]rule{}, byTo: map[string]rule{}}
	trs, err := s.jira.WorkflowTransitions(ctx, projectID, typeID)
	if err != nil {
		log.Printf("move_fields: workflow rules of %s unavailable, using screen fields + Jira's answer only: %v", key, err)
	}
	for _, tr := range trs {
		r := parseRules(tr)
		if len(r.ids) == 0 {
			continue
		}
		c.byID[tr.ID] = r
		if tr.ToStatusReference != "" {
			c.byTo[tr.ToStatusReference] = mergeRule(c.byTo[tr.ToStatusReference], r)
		}
	}
	s.mu.Lock()
	s.cache[key] = c
	s.mu.Unlock()
	return c
}

func parseRules(tr jira_client_dto.WorkflowTransition) rule {
	var r rule
	for _, v := range tr.Validators {
		ids, msg := v.RequiredFields()
		r = mergeRule(r, rule{ids: ids, message: msg})
	}
	return r
}

func mergeRule(a, b rule) rule {
	out := rule{ids: appendUnique(a.ids, b.ids...), message: a.message}
	if out.message == "" {
		out.message = b.message
	}
	return out
}

func appendUnique(list []string, ids ...string) []string {
	out := append([]string{}, list...)
	for _, id := range ids {
		found := false
		for _, x := range out {
			if x == id {
				found = true
				break
			}
		}
		if !found && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// Invalidate forgets the cached rules of (project, issue type).
func (s *Service) Invalidate(projectID, typeID string) {
	s.mu.Lock()
	delete(s.cache, cacheKey(projectID, typeID))
	s.mu.Unlock()
}

// plan is everything known about a move's required fields.
type plan struct {
	required []string                                   // all required field ids, in order
	screen   map[string]jira_client_dto.CreateMetaField // the transition screen's fields
	message  string
	values   jira_client_dto.IssueFieldValues
}

func (s *Service) plan(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr *Transition) (plan, error) {
	p := plan{screen: map[string]jira_client_dto.CreateMetaField{}}
	if screens, err := s.jira.ScreenTransitionsAs(ctx, cred, issueKey); err != nil {
		log.Printf("move_fields: screen fields of %s unavailable: %v", issueKey, err)
	} else {
		for _, st := range screens {
			if st.ID != tr.ID {
				continue
			}
			if tr.ToID == "" {
				tr.ToID = st.To.ID
			}
			for id, f := range st.Fields {
				p.screen[id] = f
				if f.Required {
					p.required = appendUnique(p.required, id)
				}
			}
		}
	}
	base, err := s.jira.FieldValuesAs(ctx, cred, issueKey, nil)
	if err != nil {
		return p, err
	}
	p.values = base
	c := s.rules(ctx, base.ProjectID, base.IssueTypeID)
	r, ok := c.byID[tr.ID]
	if !ok && tr.ToID != "" {
		r = c.byTo[tr.ToID]
	}
	p.required = appendUnique(p.required, r.ids...)
	p.message = r.message
	if len(p.required) > 0 {
		vals, err := s.jira.FieldValuesAs(ctx, cred, issueKey, p.required)
		if err != nil {
			return p, err
		}
		p.values = vals
	}
	return p, nil
}

func (p plan) missing() []string {
	out := []string{}
	for _, id := range p.required {
		if jira_fields.IsEmpty(p.values.Values[id]) {
			out = append(out, id)
		}
	}
	return out
}

// describe builds the form fields for ids: screen metadata first, else the
// issue's editmeta (fetched once), else an "unsupported" field.
func (s *Service) describe(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, screen map[string]jira_client_dto.CreateMetaField, ids []string) []jira_fields.Field {
	out := make([]jira_fields.Field, 0, len(ids))
	var edit map[string]jira_client_dto.CreateMetaField
	for _, id := range ids {
		meta, ok := screen[id]
		if !ok {
			if edit == nil {
				m, err := s.jira.EditMetaAs(ctx, cred, issueKey)
				if err != nil {
					log.Printf("move_fields: editmeta of %s unavailable: %v", issueKey, err)
					m = map[string]jira_client_dto.CreateMetaField{}
				}
				edit = m
			}
			meta, ok = edit[id]
		}
		if !ok {
			meta = jira_client_dto.CreateMetaField{FieldID: id, Name: id}
		}
		f := jira_fields.FromMetaOrUnsupported(meta)
		f.Required = true
		out = append(out, f)
	}
	return out
}

func defaultMessage(tr Transition) string {
	return "Isi dulu field berikut sebelum pindah ke " + tr.To + "."
}

func (s *Service) required(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr Transition, p plan, ids []string, message string) Required {
	if message == "" {
		message = defaultMessage(tr)
	}
	return Required{Transition: tr, Message: message, Fields: s.describe(ctx, cred, issueKey, p.screen, ids), IssueKey: issueKey}
}

// Requirements: the move's still-empty required fields (Fields empty when
// the move can go ahead).
func (s *Service) Requirements(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr Transition) (Required, error) {
	p, err := s.plan(ctx, cred, issueKey, &tr)
	if err != nil {
		return Required{}, err
	}
	return s.required(ctx, cred, issueKey, tr, p, p.missing(), p.message), nil
}

// Check runs before a plain move: a *FieldsRequiredError when a required
// field is empty, nil otherwise. A failing lookup is logged and lets the
// move go ahead — Jira still refuses it, and FromRejection explains.
func (s *Service) Check(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr Transition) error {
	req, err := s.Requirements(ctx, cred, issueKey, tr)
	if err != nil {
		log.Printf("move_fields: checking %s → %s failed, moving anyway: %v", issueKey, tr.To, err)
		return nil
	}
	if len(req.Fields) == 0 {
		return nil
	}
	return &FieldsRequiredError{Required: req}
}

// FromRejection turns Jira's 400 on a move into a *FieldsRequiredError: the
// cached rules are dropped (a board may have added one), the requirements
// re-read, and the fields Jira named added from editmeta.
func (s *Service) FromRejection(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr Transition, rej *jira_client_service.RejectedError) error {
	var p plan
	if base, err := s.jira.FieldValuesAs(ctx, cred, issueKey, nil); err == nil {
		s.Invalidate(base.ProjectID, base.IssueTypeID)
		if fresh, err := s.plan(ctx, cred, issueKey, &tr); err == nil {
			p = fresh
		}
	}
	ids := p.missing()
	for id := range rej.Fields {
		ids = appendUnique(ids, id)
	}
	message := strings.Join(rej.Messages, " ")
	if message == "" {
		message = p.message
	}
	if message == "" && len(rej.Fields) > 0 {
		for _, m := range rej.Fields {
			message = m
			break
		}
	}
	if message == "" && len(ids) == 0 {
		message = "Jira menolak perpindahan ke " + tr.To + "."
	}
	return &FieldsRequiredError{Required: s.required(ctx, cred, issueKey, tr, p, ids, message)}
}

// Apply fills values (form values keyed by field id, as Buat Card sends
// them) and then moves: fields on the transition screen ride along with the
// transition, the others are set first with an edit. Errors: a
// *ValidationError for values that can't be sent, a *FieldsRequiredError
// when a required field is still empty (or Jira refuses the move).
func (s *Service) Apply(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, tr Transition, values map[string]any) error {
	p, err := s.plan(ctx, cred, issueKey, &tr)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	fields := s.describe(ctx, cred, issueKey, p.screen, ids)
	edit, screen := map[string]any{}, map[string]any{}
	bad := map[string]string{}
	for _, f := range fields {
		if f.Kind == jira_fields.KindUnsupported {
			bad[f.ID] = "isi di Jira"
			continue
		}
		v, ok, err := jira_fields.Encode(f, values[f.ID])
		if err != nil {
			bad[f.ID] = err.Error()
			continue
		}
		if !ok {
			continue
		}
		if _, onScreen := p.screen[f.ID]; onScreen {
			screen[f.ID] = v
		} else {
			edit[f.ID] = v
		}
	}
	if len(bad) > 0 {
		return &ValidationError{Fields: bad}
	}
	var still []string
	for _, id := range p.missing() {
		_, inEdit := edit[id]
		_, inScreen := screen[id]
		if !inEdit && !inScreen {
			still = append(still, id)
		}
	}
	if len(still) > 0 {
		return &FieldsRequiredError{Required: s.required(ctx, cred, issueKey, tr, p, still, p.message)}
	}
	if len(edit) > 0 {
		if err := s.jira.EditIssueAs(ctx, cred, issueKey, edit); err != nil {
			if rej := jira_client_service.AsRejected(err); rej != nil && len(rej.Fields) > 0 {
				return &ValidationError{Fields: rej.Fields}
			}
			return err
		}
	}
	if err := s.jira.TransitionWithFieldsAs(ctx, cred, issueKey, tr.ID, screen); err != nil {
		if rej := jira_client_service.AsRejected(err); rej != nil {
			return s.FromRejection(ctx, cred, issueKey, tr, rej)
		}
		return err
	}
	return nil
}
