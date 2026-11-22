package main

import (
	"flag"
	"log"
	"net/http"

	"platform/server/internal/api"
	"platform/server/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "监听地址")
	db := flag.String("db", "data/platform.db", "SQLite 路径")
	web := flag.String("web", "", "静态前端目录（可选）")
	flag.Parse()

	st, err := store.Open(*db)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	server := api.New(st)
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
