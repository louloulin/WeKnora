package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// StringList is a JSON-array-backed string list, stored as jsonb on
// Postgres and TEXT on SQLite via the driver.Valuer / sql.Scanner
// methods. Used for columns like MFA RecoveryCodes that hold a
// slice of opaque hashes the application owns.
type StringList []string

// Value implements driver.Valuer.
func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (s *StringList) Scan(src interface{}) error {
	if src == nil {
		*s = nil
		return nil
	}
	switch v := src.(type) {
	case string:
		if v == "" {
			*s = nil
			return nil
		}
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	default:
		return errors.New("StringList.Scan: unsupported type")
	}
}
