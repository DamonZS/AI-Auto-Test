package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"platform/server/internal/api"
	"platform/server/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "监听地址")
	db := flag.String("db", "data/platform.db", "SQLite 路径")
	web := flag.String("web", "", "静态前端目录（可选）")
	provisionUser := flag.Bool("provision-user", false, "配置本地账号和租户角色后退出")
	authTenant := flag.String("auth-tenant", "", "配置账号关联的租户 ID")
	authTenantName := flag.String("auth-tenant-name", "", "配置账号时创建的租户名称（可选）")
	authRole := flag.String("auth-role", "owner", "配置账号的角色：owner、editor 或 viewer")
	flag.Parse()

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
	server := api.New(st, api.AuthOptions{})
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
