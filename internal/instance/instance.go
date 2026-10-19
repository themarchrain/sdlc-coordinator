package instance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

var id = newID()

// ID 返回本进程的实例身份（稳定于进程生命周期）。
func ID() string { return id }

func newID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "host"
	}
	// 预算：host(16) + '-' + pid(≤10) + '-' + 8 hex = ≤36，加上租约 owner 的
	// 通道后缀（":<source>"）仍不超 owner 列 64 字符上限。
	if len(host) > 16 {
		host = host[:16]
	}
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), randomSuffix())
}

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 熵源失败退化为 pid 派生的确定串：身份唯一性由进程生命周期内
		// 稳定、跨进程大概率不同承担，退化为低质量身份不阻塞启动。
		return fmt.Sprintf("%08x", os.Getpid())
	}
	return hex.EncodeToString(buf[:])
}
