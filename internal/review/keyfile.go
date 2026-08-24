package review

// Passphrase-protected Ed25519 key files (plans12 P6-3a).
//
// Private keys NEVER live unencrypted on disk and are never needed by any MCP
// process: only the human TTY channel decrypts them. The public key is stored
// in the clear inside the key file AND published to the reviewers ring that
// DocTrust verifies against.
//
// Container: JSON { kdf: pbkdf2-hmac-sha256 (600k iterations), salt,
// aes-256-gcm ciphertext of the ed25519 private key }.
// Stdlib only — no new module dependencies.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	pbkdf2Iterations = 600000
	kdfName          = "pbkdf2-hmac-sha256"
	containerVersion = 1
)

type encryptedKeyFile struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"` // base64(std)
	KeyID      string `json:"key_id"`
	Alg        string `json:"alg"`
	PublicKey  string `json:"public_key"` // base64(std) ed25519.PublicKey
	Nonce      string `json:"nonce"`      // base64(std) AES-GCM nonce
	Ciphertext string `json:"ciphertext"` // base64(std) AES-256-GCM(private key)
}

// pbkdf2Sha256 implements PBKDF2 (RFC 8018) with HMAC-SHA256. Stdlib only:
// crypto/hmac + crypto/sha256 + encoding/binary.
func pbkdf2Sha256(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var out []byte
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func deriveKey(passphrase string, salt []byte, iterations int) []byte {
	return pbkdf2Sha256([]byte(passphrase), salt, iterations, 32)
}

// GenerateReviewerKeyPair creates a fresh Ed25519 pair.
func GenerateReviewerKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// SaveEncryptedPrivateKey writes the passphrase-protected container. The
// passphrase is never persisted anywhere.
func SaveEncryptedPrivateKey(path, keyID string, priv ed25519.PrivateKey,
	passphrase string) (ed25519.PublicKey, error) {
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt, pbkdf2Iterations)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, priv, nil)

	data, err := json.MarshalIndent(encryptedKeyFile{
		Version:    containerVersion,
		KDF:        kdfName,
		Iterations: pbkdf2Iterations,
		Salt:       b64(salt),
		KeyID:      keyID,
		Alg:        AlgEd25519,
		PublicKey:  b64(pub),
		Nonce:      b64(nonce),
		Ciphertext: b64(ct),
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return pub, nil
}

// LoadEncryptedPrivateKey decrypts and returns the key pair after verifying
// the passphrase (GCM auth tag fails closed on a wrong passphrase).
func LoadEncryptedPrivateKey(path, passphrase string) (string, ed25519.PublicKey,
	ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, err
	}
	var kf encryptedKeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return "", nil, nil, fmt.Errorf("parse key file: %w", err)
	}
	if kf.KDF != kdfName || kf.Alg != AlgEd25519 {
		return "", nil, nil, fmt.Errorf("unsupported key file (kdf=%s alg=%s)", kf.KDF, kf.Alg)
	}
	salt, err := unb64(kf.Salt)
	if err != nil {
		return "", nil, nil, fmt.Errorf("salt: %w", err)
	}
	ct, err := unb64(kf.Ciphertext)
	if err != nil {
		return "", nil, nil, fmt.Errorf("ciphertext: %w", err)
	}
	nonce, err := unb64(kf.Nonce)
	if err != nil {
		return "", nil, nil, fmt.Errorf("nonce: %w", err)
	}
	pub, err := unb64(kf.PublicKey)
	if err != nil {
		return "", nil, nil, fmt.Errorf("public key: %w", err)
	}
	key := deriveKey(passphrase, salt, kf.Iterations)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, nil, err
	}
	priv, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", nil, nil, errors.New("wrong passphrase or corrupted key file")
	}
	if len(priv) != ed25519.PrivateKeySize {
		return "", nil, nil, fmt.Errorf("decrypted key has wrong size %d", len(priv))
	}
	return kf.KeyID, ed25519.PublicKey(pub), ed25519.PrivateKey(priv), nil
}

// base64 std helpers (kept local to avoid adding encoding deps to callers).
func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func unb64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
