package storage

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"uw/uboot"
	"uw/ulog"

	"nrat/pkg/nostr"

	"nrat/model"
)

var (
	storagePath string = ".control.json"
	//go:embed default.json
	defaultCfgJson []byte
)

func StorageUint(c *uboot.Context) (e error) {
	s := &Storage{
		storageData: &model.ControlStorageData{},
	}

	if e := s.Read(); e != nil {
		ulog.Warn("read storage failed: %s", e)
	}

	c.Printf("public key: %s", s.Storage().PublicKey)

	c.Set("storage", s)
	return nil
}

type Storage struct {
	storageData *model.ControlStorageData
}

func (s *Storage) Storage() *model.ControlStorageData {
	return s.storageData
}

func (s *Storage) Unostr() *model.UnostrStorageData {
	return s.storageData.UnostrStorageData
}

func (s *Storage) Write() error {
	b, e := json.MarshalIndent(s.storageData, "", "    ")
	if e != nil {
		return fmt.Errorf("marshal cfg failed: %s", e)
	}

	if e := os.WriteFile(storagePath, b, 0o644); e != nil {
		return fmt.Errorf("write storage file failed: %s", e)
	}

	return nil
}

func (s *Storage) Read() error {
	b, e := os.ReadFile(storagePath)
	if e != nil && !os.IsNotExist(e) {
		return fmt.Errorf("read storage file failed: %s", e)
	}

	b = bytes.TrimSpace(b)

	if len(b) < 1 || !bytes.HasPrefix(b, []byte("{")) ||
		!bytes.HasSuffix(b, []byte("}")) {
		b = defaultCfgJson
	}

	if e := json.Unmarshal(b, s.storageData); e != nil {
		return fmt.Errorf("unmarshal storage file failed: %s", e)
	}

	if strings.TrimSpace(s.storageData.PrivateKey) == "" {
		s.storageData.PrivateKey = nostr.GeneratePrivateKey()
		if e := s.Write(); e != nil {
			return fmt.Errorf("write cfg failed: %s", e)
		}

		ulog.Info("generated private key: %s", s.storageData.PrivateKey)
	}

	if s.storageData.PublicKey, e = nostr.
		GetPublicKey(s.storageData.PrivateKey); e != nil {
		return fmt.Errorf("get public key failed: %s", e)
	}

	return s.Write()
}
