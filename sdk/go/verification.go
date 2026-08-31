package weknora

import (
	"context"

)

// VerificationService exposes AI Verification (Build #29).
type VerificationService struct{ c *Client }

// NewVerificationService constructs a VerificationService.
func NewVerificationService(c *Client) *VerificationService {
	return &VerificationService{c: c}
}

// Verify runs an AI Verification pass and returns the report.
func (s *VerificationService) Verify(ctx context.Context, kbID string, req  VerificationRequest) (* VerificationReport, error) {
	var out  VerificationReport
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/verify", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
