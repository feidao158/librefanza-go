package main

import (
	"net/url"
	"testing"
)

func TestDatabaseURLPreservesCredentials(t *testing.T) {
	value := databaseURL("postgres", "数据库", "user", "a:b@c/#")
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Host != "postgres:5432" || password != "a:b@c/#" || parsed.Query().Get("sslmode") != "disable" {
		t.Fatal("数据库连接地址编码错误")
	}
}
