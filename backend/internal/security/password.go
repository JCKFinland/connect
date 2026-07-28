package security

import "golang.org/x/crypto/bcrypt"

const PasswordCost = bcrypt.DefaultCost

// HashPassword hashes a plain-text password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		PasswordCost,
	)

	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword verifies a plain-text password against a bcrypt hash.
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword(
		[]byte(hash),
		[]byte(password),
	)
}
