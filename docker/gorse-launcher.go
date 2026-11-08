package main

import (
	"fmt"
	"net/url"
	"os"
	"syscall"
)

func databaseURL(host, name, user, password string) string {
	return (&url.URL{Scheme: "postgres", Host: host + ":5432", Path: "/" + name, User: url.UserPassword(user, password), RawQuery: "sslmode=disable"}).String()
}

func main() {
	host, name, user, password := os.Getenv("DB_HOST"), os.Getenv("DB_NAME"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")
	if host == "" || name == "" || user == "" || password == "" {
		fmt.Fprintln(os.Stderr, "数据库配置不完整")
		os.Exit(1)
	}
	dsn := databaseURL(host, name, user, password)
	_ = os.Setenv("GORSE_DATA_STORE", dsn)
	_ = os.Setenv("GORSE_CACHE_STORE", dsn)
	if err := syscall.Exec("/usr/bin/gorse-in-one", []string{"gorse-in-one", "-c", "/etc/gorse/config.toml", "--access-log-path", "/dev/null"}, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "无法启动推荐服务")
		os.Exit(1)
	}
}
