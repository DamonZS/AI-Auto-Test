package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/netip"
	"os"

	"platform/server/internal/api"
	"platform/server/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "监听地址")
	db := flag.String("db", "data/platform.db", "SQLite 路径")
	web := flag.String("web", "", "静态前端目录（可选）")
	desktopSetup := flag.Bool("desktop-setup", false, "启用桌面首次注册（仅限回环监听）")
	provisionUser := flag.Bool("provision-user", false, "配置本地账号和租户角色后退出")
	authTenant := flag.String("auth-tenant", "", "配置账号关联的租户 ID")
	authTenantName := flag.String("auth-tenant-name", "", "配置账号时创建的租户名称（可选）")
	authRole := flag.String("auth-role", "owner", "配置账号的角色：owner、editor 或 viewer")
	flag.Parse()

	if *desktopSetup && !*provisionUser {
		listen, err := netip.ParseAddrPort(*addr)
		if err != nil || listen.Addr().Zone() != "" || !listen.Addr().Unmap().IsLoopback() {
			log.Fatal("-desktop-setup 要求 -addr 使用 IPv4 或 IPv6 回环 IP 和端口")
		}
	}

	st, err := store.Open(*db)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	if *provisionUser {
		if err := api.ProvisionUser(context.Background(), st, os.Getenv("PLATFORM_AUTH_USERNAME"), os.Getenv("PLATFORM_AUTH_PASSWORD"), *authTenant, *authTenantName, *authRole); err != nil {
			log.Fatal(err)
		}
		log.Print("本地账号和租户角色已配置")
		return
	}
	server := api.New(st, api.AuthOptions{DesktopSetup: *desktopSetup})
	server.StartScheduleLoop()
	h := routes(server.Routes(), *web)
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, h))
}

func routes(apiHandler http.Handler, webPath string) http.Handler {
	if webPath == "" {
		return apiHandler
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(webPath)))
	mux.Handle("/api/", apiHandler)
	mux.Handle("/mcp", apiHandler)
	return mux
}
