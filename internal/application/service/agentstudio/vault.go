// Package agentstudio implements the v0.7.21 Custom Agent Studio (飞书妙搭 /
// Notion Custom Agents parity). Three sub-services compose the public
// AgentStudioService surface:
//
//   vault.go    — credential encryption + secret rotation
//   quota.go    — quota enforcement + ledger append
//   trigger.go  — cron / event / webhook firing + payload templating
//
// The AgentStudioService struct lives in service.go and is wired into the
// container in a single Provide() block (see internal/container/container.go).
package agentstudio

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Vault errors. Typed so callers can branch on them.
var (
	ErrVaultNotFound       = errors.New("agentstudio.vault: credential not found")
	ErrVaultAlreadyExists  = errors.New("agentstudio.vault: credential name already exists")
	ErrVaultInvalidType    = errors.New("agentstudio.vault: invalid credential type")
	ErrVaultDecryptFailed  = errors.New("agentstudio.vault: decrypt failed (tampered or wrong key)")
	ErrVaultExpired        = errors.New("agentstudio.vault: credential expired")
)

// defaultVaultKey is a per-process placeholder key. In production the key
// is loaded from KMS via KMSResolver (Build #30+ dependency) — the
// placeholder is only used when KMS is not configured (dev profile).
// The 32-byte length matches AES-256.
var defaultVaultKey = []byte("wk-agentstudio-dev-key!!v0!!32!!")

// vaultKeyResolver returns the active AES key. Thread-safe via atomic swap.
type vaultKeyResolver struct {
	mu  sync.RWMutex
	key []byte
}

// KMSResolver can be plugged in via NewAgentStudioService to swap the
// active encryption key without restarting the process. Production
// code wires this to Tencent Cloud KMS / Aliyun KMS / HashiCorp Vault.
type KMSResolver interface {
	CurrentKey(ctx context.Context) ([]byte, error)
}

func newVaultKeyResolver() *vaultKeyResolver {
	return &vaultKeyResolver{key: defaultVaultKey}
}

func (v *vaultKeyResolver) Current() []byte {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]byte, len(v.key))
	copy(out, v.key)
	return out
}

func (v *vaultKeyResolver) Rotate(newKey []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.key = newKey
}

// Vault manages the lifecycle of agent_credentials. Encrypts at rest with
// AES-256-GCM; exposes plaintext only via Reveal(), never over the wire.
type Vault struct {
	repo   typesRepo
	keyBox *vaultKeyResolver
}

// NewVault wires the vault to the repo + key box.
func NewVault(repo typesRepo, keyBox *vaultKeyResolver) *Vault {
	return &Vault{repo: repo, keyBox: keyBox}
}

// Create encrypts the plaintext secret and stores ciphertext + nonce +
// auth_tag in agent_credentials. Returns the persisted row (without
// ciphertext).
func (v *Vault) Create(ctx context.Context, tenantID, createdBy uint64,
	name, credentialType string, plaintext []byte, expiresAt *time.Time,
) (*types.AgentCredential, error) {
	if !validCredType(credentialType) {
		return nil, ErrVaultInvalidType
	}
	if existing, err := v.repo.GetCredential(ctx, tenantID, name); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrVaultAlreadyExists
	}
	key := v.keyBox.Current()
	ciphertext, nonce, tag, err := encryptGCM(key, plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault.encrypt: %w", err)
	}
	meta, _ := json.Marshal(map[string]string{
		"alg":     "AES-256-GCM",
		"ver":     "1",
		"created": time.Now().UTC().Format(time.RFC3339Nano),
	})
	cred := &types.AgentCredential{
		TenantID:       tenantID,
		Name:           name,
		CredentialType: credentialType,
		Ciphertext:     ciphertext,
		Nonce:          nonce,
		AuthTag:        tag,
		EncMeta:        string(meta),
		CreatedBy:      createdBy,
		ExpiresAt:      expiresAt,
	}
	if err := v.repo.CreateCredential(ctx, cred); err != nil {
		return nil, err
	}
	return cred, nil
}

// Reveal decrypts and returns the plaintext secret. Touches the
// last_used_at column for observability. Returns ErrVaultExpired if
// the credential has passed its expires_at.
func (v *Vault) Reveal(ctx context.Context, tenantID uint64, name string) ([]byte, error) {
	cred, err := v.repo.GetCredential(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, ErrVaultNotFound
	}
	if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
		return nil, ErrVaultExpired
	}
	key := v.keyBox.Current()
	plain, err := decryptGCM(key, cred.Ciphertext, cred.Nonce, cred.AuthTag)
	if err != nil {
		return nil, ErrVaultDecryptFailed
	}
	if err := v.repo.TouchCredentialUsage(ctx, tenantID, name); err != nil {
		logger.Warnf(ctx, "[Vault] touch usage failed: %v", err)
	}
	return plain, nil
}

// RevealAsHeader returns the credential formatted as an HTTP header value
// (e.g. "Bearer xxxxx" or "Basic base64"). Saves tool-binding code from
// repeating the same logic.
func (v *Vault) RevealAsHeader(ctx context.Context, tenantID uint64, name string) (string, error) {
	cred, err := v.repo.GetCredential(ctx, tenantID, name)
	if err != nil {
		return "", err
	}
	if cred == nil {
		return "", ErrVaultNotFound
	}
	plain, err := v.Reveal(ctx, tenantID, name)
	if err != nil {
		return "", err
	}
	switch cred.CredentialType {
	case types.AgentCredTypeBearer:
		return "Bearer " + string(plain), nil
	case types.AgentCredTypeBasic:
		return "Basic " + base64.StdEncoding.EncodeToString(plain), nil
	case types.AgentCredTypeAPIKey:
		return string(plain), nil
	default:
		return string(plain), nil
	}
}

// Delete removes the credential. Idempotent — returns nil on missing.
func (v *Vault) Delete(ctx context.Context, tenantID uint64, name string) error {
	return v.repo.DeleteCredential(ctx, tenantID, name)
}

// List returns metadata only (no ciphertext). Wire-safe.
func (v *Vault) List(ctx context.Context, tenantID uint64) ([]*types.AgentCredential, error) {
	return v.repo.ListCredentials(ctx, tenantID)
}

// --- crypto primitives ---

func encryptGCM(key, plaintext []byte) (ct, nonce, tag []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	ct = sealed[:len(sealed)-16]
	tag = sealed[len(sealed)-16:]
	return ct, nonce, tag, nil
}

func decryptGCM(key, ct, nonce, tag []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := make([]byte, 0, len(ct)+len(tag))
	sealed = append(sealed, ct...)
	sealed = append(sealed, tag...)
	return gcm.Open(nil, nonce, sealed, nil)
}

func validCredType(t string) bool {
	switch strings.ToLower(t) {
	case types.AgentCredTypeAPIKey,
		types.AgentCredTypeOAuth2,
		types.AgentCredTypeBasic,
		types.AgentCredTypeBearer,
		types.AgentCredTypeCustom:
		return true
	}
	return false
}
