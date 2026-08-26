package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

type config struct {
	addr      string
	database  string
	selfCheck bool
}

func parseConfig() (config, error) {
	var c config
	flag.StringVar(&c.addr, "addr", "127.0.0.1:19091", "HTTP 监听地址")
	flag.StringVar(&c.database, "db", "wellseal.db", "SQLite 数据库路径")
	flag.BoolVar(&c.selfCheck, "self-check", false, "执行真实 HTTP 全流程自检后退出")
	flag.Parse()
	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			explicit = true
		}
	})
	if !explicit {
		if p := os.Getenv("PORT"); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return c, fmt.Errorf("PORT 必须为有效端口号")
			}
			c.addr = net.JoinHostPort("127.0.0.1", p)
		}
	}
	host, port, err := net.SplitHostPort(c.addr)
	if err != nil {
		return c, fmt.Errorf("-addr 格式无效: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("监听端口无效")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return c, fmt.Errorf("监听地址必须是回环地址")
	}
	return c, nil
}
