# Nrat

> 一个基于 Nostr 去中心的匿名远程控制工具

> A decentralized anonymous remote control tool based on Nostr

> Децентрализованный анонимный инструмент удаленного управления на основе Nostr

## 介绍

Nrat 是一个基于 Nostr 去中心的匿名远程控制工具, 使用 Nostr 的匿名通信特性, 使得 Nrat 可以在不暴露服务器 IP 的情况下进行远程控制.

并且由于愈发健壮的 Nostr 网络, 在全球范围内的节点都可以进行通信, 相较于传统的 IRC 中继网络, Nrat 的通信更加稳定, 延迟更低, 并且支持非对称加密, 使得通信更加安全.

Nrat 由两个部分组成, 一个是控制端, 一个是被控端.

control: 控制端用于控制被控端, 并且在没有 Golang 语言环境的情况下修补被控端二进制嵌入配置文件数据.
agent: 被控端用于接收控制端的指令, 并定期通过 meta 广播自身的信息, 以便控制端发现.

## 功能

1. 文件管理 (文件增删改查)
2. 远程执行命令
3. 剪切板
4. 截图 (WIP)

## 使用

### 控制端

编译或者在 [Release](https://github.com/ClarkQAQ/nrat/releases) 中下载控制端二进制文件, 然后运行即可.

### 被控端

编译或者在 [Release](https://github.com/ClarkQAQ/nrat/releases) 中下载被控端二进制文件, 然后使用控制端的 `fix <input file path> <output file path>` 命令修补并嵌入配置文件进被控端二进制文件, 最后运行被控端即可, 被控端会自动连接 Nostr 网络并广播自身的信息. 并且被控端密钥也会被写入控制端的配置文件中, 以便控制端连接被控端.

## 指令

1. `help`: 显示帮助信息
2. `fix <input file path> <output file path>`: 修补被控端二进制文件并嵌入配置文件
3. `agent`: 显示配置文件中的被控端信息
4. `connect | cc <agent id>`: 选择或者直接连接被控端
5. `list | ls <path>`: 列出被控端当前的文件列表
6. `chdir | cd <path>`: 切换被控端当前的目录
7. `mkdir <path>`: 在被控端当前的目录下创建目录
8. `remove | rm <path>`: 删除被控端当前的目录或者文件
9. `move | mv <old path> <new path>`: 重命名被控端当前的目录或者文件
10. `upload | up <local file path> <remote file path>`: 上传本地文件到被控端
11. `download | dl <remote file path> <local file path>`: 下载被控端文件到本地
12. `exec <command>`: 在被控端执行命令
13. `info`: 显示被控端信息, 添加任意参数显示完整私钥

## 最后

Happy Hacking!
