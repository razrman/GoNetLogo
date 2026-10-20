// Command server is the GraphQL API for GoNetLogo.
// It owns the interpreter, the world, and one session per client.
// Built with -tags web, it also serves the React UI on the same port.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"

	"gonetlogo/backend/graph"
	"gonetlogo/backend/session"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:43131", "listen address")
	webDir := flag.String("web", "", "directory of a built frontend (overrides files embedded with -tags web)")
	flag.Parse()

	store := session.NewStore()
	if dir, err := session.ModelsDir(); err != nil {
		log.Printf("model files: %v (embedded demos still load)", err)
	} else {
		log.Printf("model files confined to %s", dir)
	}
	srv := newHandler(store)

	ui, err := openUI(*webDir)
	if err != nil {
		log.Fatalf("web UI: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/graphql", srv)
	if ui != nil {
		// The app owns /. The playground stays available beside it.
		mux.Handle("/", spaHandler(ui))
		mux.Handle("/playground", playground.Handler("GoNetLogo GraphQL", "/graphql"))
		log.Printf("web UI at http://%s/", *addr)
		log.Printf("GraphQL playground at http://%s/playground", *addr)
	} else {
		mux.Handle("/", playground.Handler("GoNetLogo GraphQL", "/graphql"))
		log.Printf("GraphQL playground at http://%s/", *addr)
	}
	log.Printf("GraphQL at http://%s/graphql", *addr)
	log.Fatal(http.ListenAndServe(*addr, cors(mux)))
}

func newHandler(store *session.Store) http.Handler {
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{Store: store}}))
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New(256))
	srv.Use(extension.Introspection{})
	return srv
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
