package model

type UnostrStorageData struct {
	Relay          string `json:"relay"`           // 中继器
	Proxy          string `json:"proxy"`           // 代理
	ConnectTimeout string `json:"connect_timeout"` // 连接超时
	PingInterval   string `json:"ping_interval"`   // ping间隔
}

type UnostrStorage interface {
	Unostr() *UnostrStorageData
	Write() error
	Read() error
}

type AgentStorageData struct {
	*UnostrStorageData
	PrivateKey        string `json:"private_key"`        // 私钥
	BroadcastInterval string `json:"broadcast_interval"` // 广播间隔
	PublicKey         string `json:"-"`                  // 公钥
}

type ControlStorageData struct {
	*UnostrStorageData
	PrivateKey          string   `json:"private_key"`            // 私钥
	AgentPrivateKeyList []string `json:"agent_private_key_list"` // 客户端私钥列表
	PublicKey           string   `json:"-"`                      // 公钥
	CmdTimeout          string   `json:"cmd_timeout"`            // 命令等待超时
	HistoryFile         string   `json:"history_file"`           // 历史文件
	ExecTimeout         string   `json:"exec_timeout"`           // 远程命令执行超时
}

type Storage[T any] interface {
	Storage() T
	Write() error
	Read() error
}
