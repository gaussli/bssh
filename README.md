# bssh

批量 SSH 命令执行工具：从一个 YAML 主机清单读取目标主机，启动阶段先并发建立所有主机连接，连接完成后再逐行读取命令，并在所有主机上串行执行，输出统一带上主机标识前缀。

适合在少量机器上做批量巡检、信息收集，例如一次查看多台机器的负载、磁盘或版本信息。

## 特性

- 启动时并发连接所有主机并逐台显示进度，连接全部完成后才允许输入命令，避免输入后等待 SSH 握手
- 连接失败的主机自动跳过，不会拖慢后续命令；全部连接失败时直接报错退出
- 交互式逐行输入命令，输入 `exit` 或 EOF 结束
- 每台主机复用同一条 SSH 连接，每条命令新建 session，避免重复握手
- 公钥认证（`~/.ssh/id_ed25519`、`~/.ssh/id_rsa`、`~/.ssh/id_ecdsa`，按此顺序取第一个存在的未加密私钥）
- 基于 `~/.ssh/known_hosts` 校验主机密钥，连接超时 10 秒
- 输出格式为 `user@host:port<TAB>输出行`，单台主机失败不影响其他主机

## 环境要求

- Go 1.27.4 或更高版本
- `~/.ssh/` 下存在可用的私钥（暂不支持带口令的加密私钥）
- 目标主机的公钥已写入 `~/.ssh/known_hosts`，例如：

```bash
ssh-keyscan -p 22 192.168.1.1 >> ~/.ssh/known_hosts
```

## 安装与构建

```bash
go install github.com/gaussli/bssh@latest
```

或从源码构建：

```bash
git clone git@github.com:gaussli/bssh.git
cd bssh
go build -o bssh .
```

## 使用

准备主机清单 `hosts.yaml`：

```yaml
hosts:
  - host: 192.168.1.1
  - host: 192.168.1.2
    user: deploy
    port: 2222
```

`hosts` 列表每项字段：

| 字段 | 含义 | 默认值 |
| :--- | :--- | :--- |
| `host` | 目标主机地址，必填 | — |
| `user` | SSH 用户名 | `root` |
| `port` | SSH 端口，范围 1–65535 | `22` |

字段名严格校验，出现未知字段（如把 `port` 写成 `prot`）会直接报错。

启动并输入命令：

```bash
./bssh -f hosts.yaml
uptime
df -h /
exit
```

启动阶段的连接进度输出到 stderr：

```
正在连接 root@192.168.1.1:22 ...
正在连接 deploy@192.168.1.2:2222 ...
已连接 root@192.168.1.1:22
已连接 deploy@192.168.1.2:2222
已连接 2/2 台主机，可以输入命令（exit 退出）
```

结果按连接完成的先后显示；连接失败的主机打印 `连接失败 user@host:port: 原因`，并从后续命令的目标中剔除，避免每条命令都卡在不可达主机上。

命令输出示例：

```
root@192.168.1.1:22	 10:00:00 up 10 days,  1:00,  1 user,  load average: 0.10, 0.20, 0.15
deploy@192.168.1.2:2222	 10:00:00 up 5 days,  2:30,  0 users,  load average: 0.01, 0.03, 0.00
```

命令也可以由管道提供，此时读完后自动退出：

```bash
echo 'hostname' | ./bssh -f hosts.yaml
```

执行失败时，错误以 `user@host:port<TAB>ERROR: ...` 输出到 stderr，其余主机继续执行。连接进度与失败提示同样走 stderr，不会混入 stdout 的命令结果。

## 说明与限制

- 命令在主机之间串行执行，不并发；启动阶段的连接建立是并发的，同一时刻只维护一条到每台主机的连接
- 启动时连接失败的主机不参与后续命令，修复后需重新启动 `bssh`
- 仅收集远端命令的 stdout；失败时把远端 stderr 附加在错误信息中
- 命令在远端非交互执行，不适合需要 TTY 或交互输入的程序
- 每次连接都会校验 `known_hosts`，未知主机会被拒绝

## 开发与测试

```bash
go test ./...
go vet ./...
```

目录结构：

```
main.go                     程序入口
cmd/                        Cobra 命令定义、连接预热与交互式命令循环
internal/hosts/             YAML 主机清单解析
internal/sshclient/         SSH 认证、连接复用与命令执行
```

## 许可证

[MIT](LICENSE)
