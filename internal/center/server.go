package center

import (
	"net/http"
)

func StartServer(addr string) error{
	mux := http.NewServeMux()

	mux.HandleFunc("/heartbeat", HeartbeatHandler)

	return http.ListenAndServe(addr, mux)
}
