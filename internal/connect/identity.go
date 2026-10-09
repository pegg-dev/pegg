package connect

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
)

type Identity struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`
	PrivatePEM string `json:"private_key"`
	CreatedAt  string `json:"created_at"`

	priv ed25519.PrivateKey
}

func LoadOrCreateIdentity(name string) (*Identity, error) {
	path, err := devicePath()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(path); err == nil {
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, fmt.Errorf("connect: parse device identity: %w", err)
		}
		if err := id.parseKey(); err != nil {
			return nil, err
		}
		if name != "" && name != id.Name {
			id.Name = name
			if err := id.save(path); err != nil {
				return nil, err
			}
		}
		return &id, nil
	}

	id, err := newIdentity(name)
	if err != nil {
		return nil, err
	}
	if err := id.save(path); err != nil {
		return nil, err
	}
	return id, nil
}

func newIdentity(name string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("connect: generate device key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("connect: marshal device key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return &Identity{
		DeviceID:   uuid.NewString(),
		Name:       name,
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivatePEM: string(pemBytes),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		priv:       priv,
	}, nil
}

func (id *Identity) parseKey() error {
	block, _ := pem.Decode([]byte(id.PrivatePEM))
	if block == nil {
		return fmt.Errorf("connect: invalid device private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("connect: parse device private key: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("connect: device key is not ed25519")
	}
	id.priv = priv
	return nil
}

func (id *Identity) save(path string) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("connect: marshal device identity: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("connect: write device identity: %w", err)
	}
	return nil
}

func (id *Identity) Sign(msg []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(id.priv, msg))
}

func helloMessage(deviceID, nonce string, ts int64) []byte {
	return []byte("pegg-connect-hello:" + deviceID + ":" + nonce + ":" + strconv.FormatInt(ts, 10))
}

func ResetIdentity() error {
	path, err := devicePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func HelloMessage(deviceID, nonce string, ts int64) []byte {
	return helloMessage(deviceID, nonce, ts)
}

func VerifyHello(publicKeyB64, deviceID, nonce string, ts int64, sigB64 string) error {
	pub, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return fmt.Errorf("connect: decode public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("connect: invalid public key length")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("connect: decode signature: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), helloMessage(deviceID, nonce, ts), sig) {
		return fmt.Errorf("connect: invalid hello signature")
	}
	return nil
}
