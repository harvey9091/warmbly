package cipher

import (
	"context"

	"github.com/google/uuid"
)

// staticService hands out one fixed key for every organization.
type staticService struct{ key []byte }

// NewStatic is a CipherService over one fixed 32-byte key, for tests; production resolves a DEK per organization.
func NewStatic(key []byte) CipherService {
	return &staticService{key: key}
}

func (s *staticService) Cipher(context.Context, uuid.UUID) (*Cipher, error) {
	return &Cipher{plainDEK: s.key}, nil
}
