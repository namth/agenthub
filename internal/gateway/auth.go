package gateway

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

var ErrUnauthorized = errors.New("UNAUTHORIZED")

type Authenticator func(token string) (workerID string, err error)

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func DBAuthenticator(db *sql.DB) Authenticator {
	return func(token string) (string, error) {
		if token == "" {
			return "", ErrUnauthorized
		}
		h := HashToken(token)
		var id string
		err := db.QueryRow(`SELECT id FROM workers WHERE token_hash = ?`, h).Scan(&id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", ErrUnauthorized
			}
			return "", err
		}
		return id, nil
	}
}
