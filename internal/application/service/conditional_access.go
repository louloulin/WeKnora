package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ConditionalAccessService evaluates a tenant's enabled policies
// against an EvaluationRequest and returns the winning Decision.
//
// Algorithm (short-circuit on first match, lowest priority wins):
//
//  1. Read all enabled policies for the tenant (one indexed query).
//  2. For each policy in priority ASC order, run MatchConditions
//     against the request. The first policy whose action is not
//     "allow" wins; an "allow" policy only fires if no earlier
//     policy matched (it is the implicit default).
//  3. If no policy matched, return Decision{Action: allow} so the
//     login flow proceeds. The audit log records "no policy fired".
type ConditionalAccessService struct {
	repo interfaces.ConditionalAccessRepository
	// now is injected for tests so they can pin time-of-day / day-of-week.
	now func() time.Time
}

// NewConditionalAccessService is the DI constructor.
func NewConditionalAccessService(repo interfaces.ConditionalAccessRepository) *ConditionalAccessService {
	return &ConditionalAccessService{repo: repo, now: time.Now}
}

// SetNow overrides the clock — used by tests.
func (s *ConditionalAccessService) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// ErrConditionalAccessInvalidRequest is the service-layer sentinel
// for malformed Create / Update inputs (empty tenant_id, empty name,
// invalid action, etc.).
var ErrConditionalAccessInvalidRequest = errors.New("conditional access: invalid request")

// ErrConditionalAccessPolicyNotFound / ErrConditionalAccessPolicyExists
// are the service-layer re-exports of the repository sentinels so the
// handler only has to switch on one set of names.
var (
	ErrConditionalAccessPolicyNotFound = interfaces.ErrConditionalAccessPolicyNotFound
	ErrConditionalAccessPolicyExists   = interfaces.ErrConditionalAccessPolicyExists
)

// CreatePolicy validates inputs and persists the policy.
func (s *ConditionalAccessService) CreatePolicy(
	ctx context.Context, p *types.ConditionalAccessPolicy,
) (*types.ConditionalAccessPolicy, error) {
	if err := validatePolicy(p); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdatePolicy mutates the mutable fields of an existing policy.
func (s *ConditionalAccessService) UpdatePolicy(
	ctx context.Context, p *types.ConditionalAccessPolicy,
) (*types.ConditionalAccessPolicy, error) {
	if err := validatePolicy(p); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// DeletePolicy soft-deletes the policy. Idempotent.
func (s *ConditionalAccessService) DeletePolicy(ctx context.Context, tenantID string, id uint64) error {
	return s.repo.SoftDelete(ctx, tenantID, id)
}

// GetPolicy fetches by id for the admin UI.
func (s *ConditionalAccessService) GetPolicy(ctx context.Context, tenantID string, id uint64) (*types.ConditionalAccessPolicy, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// ListPolicies returns every live policy for the admin UI.
func (s *ConditionalAccessService) ListPolicies(ctx context.Context, tenantID string, limit, offset int) ([]*types.ConditionalAccessPolicy, int64, error) {
	return s.repo.ListAll(ctx, tenantID, limit, offset)
}

// Evaluate runs the hot read path. Returns Decision with
// Action=allow when no policy fires (the implicit default).
func (s *ConditionalAccessService) Evaluate(
	ctx context.Context, req types.EvaluationRequest,
) (types.Decision, error) {
	if req.Now.IsZero() {
		req.Now = s.now().UTC()
	} else {
		req.Now = req.Now.UTC()
	}
	policies, err := s.repo.ListEnabled(ctx, req.TenantID)
	if err != nil {
		return types.Decision{}, err
	}
	for _, p := range policies {
		if !MatchConditions(p.Conditions, req) {
			continue
		}
		return types.Decision{
			Action:          p.Action,
			Reason:          fmt.Sprintf("matched policy %q (priority %d)", p.Name, p.Priority),
			MatchedPolicyID: p.ID,
		}, nil
	}
	return types.Decision{
		Action: types.PolicyActionAllow,
		Reason: "no policy matched",
	}, nil
}

// validatePolicy enforces the minimum invariants the store relies on.
func validatePolicy(p *types.ConditionalAccessPolicy) error {
	if p == nil {
		return ErrConditionalAccessInvalidRequest
	}
	if p.TenantID == "" {
		return ErrConditionalAccessInvalidRequest
	}
	if p.Name == "" {
		return ErrConditionalAccessInvalidRequest
	}
	switch p.Action {
	case types.PolicyActionAllow, types.PolicyActionDeny, types.PolicyActionRequireMFA:
	default:
		return ErrConditionalAccessInvalidRequest
	}
	if p.Priority < 0 {
		p.Priority = 100
	}
	return nil
}

// MatchConditions is the pure function the evaluator calls against
// every policy. It is exported so the unit tests can hit it directly.
func MatchConditions(cond types.PolicyConditions, req types.EvaluationRequest) bool {
	if len(cond.IPCIDRs) > 0 {
		if !matchCIDRs(cond.IPCIDRs, req.ClientIP) {
			return false
		}
	}
	if len(cond.Countries) > 0 {
		if !condAccessContainsString(cond.Countries, strings.ToUpper(req.CountryCode)) {
			return false
		}
	}
	if len(cond.DevicePostures) > 0 {
		if !condAccessContainsString(cond.DevicePostures, req.DevicePosture) {
			return false
		}
	}
	if len(cond.Roles) > 0 {
		if !condAccessContainsString(cond.Roles, req.UserRole) {
			return false
		}
	}
	if len(cond.DaysOfWeek) > 0 {
		day := strings.ToLower(req.Now.Weekday().String())
		if !condAccessContainsString(cond.DaysOfWeek, day) {
			return false
		}
	}
	if cond.HourRange.Start != 0 || cond.HourRange.End != 0 {
		h := req.Now.Hour()
		if !inHourRange(cond.HourRange.Start, cond.HourRange.End, h) {
			return false
		}
	}
	return true
}

func matchCIDRs(cidrs []string, ip string) bool {
	if ip == "" {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func inHourRange(start, end, h int) bool {
	if start < 0 || start > 23 || end < 1 || end > 24 || start >= end {
		return false
	}
	return h >= start && h < end
}

func condAccessContainsString(haystack []string, needle string) bool {
	if needle == "" {
		return false
	}
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
