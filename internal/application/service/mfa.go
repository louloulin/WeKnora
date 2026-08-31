package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mfasp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// MFA errors surfaced to the handler.
var (
	ErrMFACredentialNotFound = repository.ErrMFACredentialNotFound
	ErrMFAAlreadyEnrolled    = errors.New("mfa: user already has an active TOTP enrolment")
	ErrMFACodeInvalid        = errors.New("mfa: code invalid")
	ErrMFARecoveryInvalid    = mfasp.ErrInvalidRecovery
	ErrMFACredentialDisabled = errors.New("mfa: credential is disabled")
)

// MFAService owns the lifecycle of a per-user MFA enrolment:
// enrollment (returns provisioning URI + recovery codes once),
// verification (TOTP or recovery code), disable.
type MFAService struct {
	repo   interfaces.MFACredentialRepository
	issuer string // deployment name surfaced in the provisioning URI
}

// NewMFAService constructs the service.
func NewMFAService(repo interfaces.MFACredentialRepository, issuer string) *MFAService {
	if issuer == "" {
		issuer = "WeKnora"
	}
	return &MFAService{repo: repo, issuer: issuer}
}

// EnrollResult is the bundle returned by Enroll.
type EnrollResult struct {
	Credential      *types.MFACredential
	ProvisioningURI string
	RecoveryCodes   []string
}

// Enroll creates a fresh TOTP enrolment for the user, persists it
// (secret encrypted at rest), and returns the URI the user scans
// in their authenticator app + the recovery scratch sheet. Both
// are shown exactly once; we never store the plaintexts.
func (s *MFAService) Enroll(ctx context.Context, userID, label string) (*EnrollResult, error) {
	if userID == "" {
		return nil, errors.New("mfa: userID is required")
	}
	if label == "" {
		return nil, errors.New("mfa: name is required")
	}
	// One active TOTP per user for now; multiple factors (TOTP +
	// WebAuthn) is a follow-up. We reject rather than silently
	// creating a second row so the UI can prompt for naming.
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range existing {
		if c.Enabled && c.Type == "totp" && c.RevokedAt == nil {
			return nil, ErrMFAAlreadyEnrolled
		}
	}
	secret, err := mfasp.GenerateSecret()
	if err != nil {
		return nil, err
	}
	codes, err := mfasp.GenerateRecoveryCodes(mfasp.DefaultRecoveryCount)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(codes))
	plains := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, c.Hash)
		plains = append(plains, c.Plain)
	}
	account := userID
	row := &types.MFACredential{
		UserID:        userID,
		Type:          "totp",
		SecretHash:    string(secret),
		RecoveryCodes: hashes,
		Name:          label,
		Enabled:       true,
		EnrolledAt:    time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, fmt.Errorf("mfa: persist: %w", err)
	}
	logger.Infof(ctx, "mfa: enrolled user %s credential %d", userID, row.ID)
	return &EnrollResult{
		Credential:      row,
		ProvisioningURI: secret.ProvisioningURI(account, s.issuer),
		RecoveryCodes:   plains,
	}, nil
}

// Verify checks a user-supplied code (TOTP or recovery) against
// the named credential. Returns nil on success, the appropriate
// sentinel on failure.
//
// We track LastUsedCounter so a replay within the ±1 step drift
// window is rejected. The recovery path additionally removes the
// consumed hash from the recovery_codes column.
func (s *MFAService) Verify(ctx context.Context, credentialID uint64, code, recovery string) error {
	cred, err := s.repo.GetByID(ctx, credentialID)
	if err != nil {
		return err
	}
	if !cred.Enabled || cred.RevokedAt != nil {
		return ErrMFACredentialDisabled
	}
	if cred.Type != "totp" {
		return fmt.Errorf("mfa: unsupported credential type %q", cred.Type)
	}
	// Recovery code path: a non-empty recoveryCode bypasses TOTP
	// verification and consumes one of the recovery hashes.
	if recovery != "" {
		idx, err := mfasp.MatchRecoveryCode(recovery, cred.RecoveryCodes)
		if err != nil {
			return ErrMFARecoveryInvalid
		}
		// Remove the consumed hash.
		cred.RecoveryCodes = append(cred.RecoveryCodes[:idx], cred.RecoveryCodes[idx+1:]...)
		now := time.Now().UTC()
		cred.LastUsedAt = &now
		return s.repo.Update(ctx, cred)
	}
	if code == "" {
		return ErrMFACodeInvalid
	}
	secret := mfasp.Secret(cred.SecretHash)
	ok, counter, err := secret.Verify(code, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMFACodeInvalid, err)
	}
	if !ok {
		return ErrMFACodeInvalid
	}
	if int64(counter) <= cred.LastUsedCounter {
		// Replay within drift window — counter must advance.
		return ErrMFACodeInvalid
	}
	cred.LastUsedCounter = int64(counter)
	now := time.Now().UTC()
	cred.LastUsedAt = &now
	return s.repo.Update(ctx, cred)
}

// Disable revokes the credential. Idempotent: disabling an
// already-disabled credential returns nil.
func (s *MFAService) Disable(ctx context.Context, credentialID uint64) error {
	cred, err := s.repo.GetByID(ctx, credentialID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	cred.Enabled = false
	cred.RevokedAt = &now
	return s.repo.Update(ctx, cred)
}

// List returns every credential for the user (metadata only — the
// secret is never returned).
func (s *MFAService) List(ctx context.Context, userID string) ([]*types.MFACredential, error) {
	return s.repo.GetByUserID(ctx, userID)
}
