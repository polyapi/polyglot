package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/hkdf"
)

const (
	appSalt        = "polyapi-polyglot-config-v1"
	appInfo        = "polyapi/polyglot/api-key/v1"
	keyringService = "polyapi"
	keyringUser    = "device-key"
)

// Compiled into the binary; combined with the device secret via HKDF.
var appMixin = [32]byte{
	0x3c, 0x91, 0xa7, 0x12, 0x58, 0x04, 0xdd, 0x6e, 0xb3, 0x4f, 0x19, 0xc2, 0x70, 0x8a, 0xe5, 0x26,
	0x11, 0x9d, 0x43, 0xfa, 0x67, 0x0b, 0xce, 0x82, 0x5f, 0x34, 0xa8, 0xd1, 0x09, 0x7c, 0xbe, 0x50,
}

// SecretContext is how to obtain the per-device secret.
type SecretContext struct {
	UserConfigDir string
	UseKeyring    bool
}

// ProductionSecrets uses the real user config dir and OS keychain.
func ProductionSecrets() SecretContext {
	return SecretContext{
		UserConfigDir: UserConfigDir(),
		UseKeyring:    true,
	}
}

// ForTests disables the keychain and uses a temp user config dir.
func ForTests(userConfigDir string) SecretContext {
	return SecretContext{
		UserConfigDir: userConfigDir,
		UseKeyring:    false,
	}
}

// DeviceSecret loads or creates the 32-byte device secret.
//
// device.key is the source of truth so encrypt/decrypt stay consistent even
// when the OS keychain does not persist. The keychain is filled when available
// as a second copy of the same bytes.
func DeviceSecret(ctx SecretContext) ([]byte, error) {
	path := DeviceKeyFile(ctx.UserConfigDir)
	if _, err := os.Stat(path); err == nil {
		return readDeviceFile(path)
	}
	if ctx.UseKeyring {
		if secret, err := keyringGet(); err == nil {
			_ = writeDeviceFile(ctx.UserConfigDir, secret)
			return secret, nil
		}
	}
	secret := random32()
	if err := writeDeviceFile(ctx.UserConfigDir, secret[:]); err != nil {
		return nil, err
	}
	if ctx.UseKeyring {
		_ = keyringSet(secret[:])
	}
	return secret[:], nil
}

// EncryptAPIKey encrypts plaintext to v1:<nonce-b64>:<ciphertext-b64>.
func EncryptAPIKey(plaintext string, deviceSecret []byte) (string, error) {
	key, err := deriveKey(deviceSecret)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", message(err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", message(err.Error())
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", message(err.Error())
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAPIKey decrypts a token produced by EncryptAPIKey.
func DecryptAPIKey(token string, deviceSecret []byte) (string, error) {
	nonceB64, ctB64, err := parseToken(token)
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return "", decryptError()
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ctB64)
	if err != nil {
		return "", decryptError()
	}
	if len(nonce) != 12 {
		return "", decryptError()
	}
	key, err := deriveKey(deviceSecret)
	if err != nil {
		return "", decryptError()
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", decryptError()
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", decryptError()
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", decryptError()
	}
	return string(plain), nil
}

// RedactSecret is the last-four redaction used by whoami / config.
func RedactSecret(secret string) string {
	if secret == "" {
		return "(unset)"
	}
	if len(secret) <= 4 {
		return "********"
	}
	return "********" + secret[len(secret)-4:]
}

func deriveKey(deviceSecret []byte) ([]byte, error) {
	info := append([]byte(appInfo), appMixin[:]...)
	r := hkdf.New(sha256.New, deviceSecret, []byte(appSalt), info)
	okm := make([]byte, 32)
	if _, err := io.ReadFull(r, okm); err != nil {
		return nil, message("HKDF expand failed")
	}
	return okm, nil
}

func parseToken(token string) (nonce, ct string, err error) {
	parts := strings.SplitN(token, ":", 3)
	if len(parts) != 3 {
		return "", "", decryptError()
	}
	if parts[0] != "v1" || parts[1] == "" || parts[2] == "" {
		return "", "", decryptError()
	}
	return parts[1], parts[2], nil
}

func keyringGet() ([]byte, error) {
	stored, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		return nil, message(err.Error())
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored))
	if err != nil {
		return nil, message("invalid keychain secret encoding")
	}
	return decoded, nil
}

func keyringSet(secret []byte) error {
	return keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(secret))
}

func readDeviceFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ioError(path, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, message("invalid device key file " + path)
	}
	return decoded, nil
}

func writeDeviceFile(userConfigDir string, secret []byte) error {
	if err := os.MkdirAll(userConfigDir, 0o700); err != nil {
		return ioError(userConfigDir, err)
	}
	path := DeviceKeyFile(userConfigDir)
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(secret)), 0o600); err != nil {
		return ioError(path, err)
	}
	return nil
}

func random32() [32]byte {
	var buf [32]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		panic(err)
	}
	return buf
}
