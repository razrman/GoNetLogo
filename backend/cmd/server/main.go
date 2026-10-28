// Command server is the GraphQL API for GoNetLogo.
// It owns the interpreter, the world, and one session per client.
// Built with -tags web, it also serves the React UI on the same port.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	_ "net/http/pprof"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"

	"gonetlogo/backend/graph"
	"gonetlogo/backend/session"
	"gonetlogo/internal/logcfg"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:43131", "listen address")
	webDir := flag.String("web", "", "directory of a built frontend (overrides files embedded with -tags web)")
	logs := logcfg.Register(flag.CommandLine)
	pprofOn := flag.Bool("pprof", false, "serve Go pprof handlers on 127.0.0.1:6060 only; off by default")
	flag.Parse()
	if _, err := logs.Apply(os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "gonetlogo-server: %v\n", err)
		os.Exit(2)
	}
	defer logcfg.Close()
	defer logcfg.Recover()
	logcfg.CloseOnSignal()

	if *pprofOn {
		ln, err := net.Listen("tcp", "127.0.0.1:6060")
		if err != nil {
			logcfg.Fail(1, "pprof listen", "addr", "127.0.0.1:6060", "err", err)
		}
		slog.Info("pprof", "url", "http://127.0.0.1:6060/debug/pprof/")
		go func() {
			if err := http.Serve(ln, nil); err != nil {
				slog.Error("pprof", "err", err)
			}
		}()
	}

	store := session.NewStore()
	if dir, err := session.ModelsDir(); err != nil {
		slog.Warn("model files unavailable; embedded demos still load", "err", err)
	} else {
		slog.Info("model files confined", "dir", dir)
	}
	srv := newHandler(store)

	ui, err := openUI(*webDir)
	if err != nil {
		logcfg.Fail(1, "web UI", "err", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/graphql", srv)
	if ui != nil {
		// The app owns /. The playground stays available beside it.
		mux.Handle("/", spaHandler(ui))
		mux.Handle("/playground", playground.Handler("GoNetLogo GraphQL", "/graphql"))
		slog.Info("web UI", "url", "http://"+*addr+"/")
		slog.Info("GraphQL playground", "url", "http://"+*addr+"/playground")
	} else {
		mux.Handle("/", playground.Handler("GoNetLogo GraphQL", "/graphql"))
		slog.Info("GraphQL playground", "url", "http://"+*addr+"/")
	}
	slog.Info("GraphQL endpoint", "url", "http://"+*addr+"/graphql")
	if err := http.ListenAndServe(*addr, logHTTP(cors(mux))); err != nil {
		logcfg.Fail(1, "listen", "addr", *addr, "err", err)
	}
}

func newHandler(store *session.Store) http.Handler {
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{Store: store}}))
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		InitFunc: func(ctx context.Context, _ transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			if slog.Default().Enabled(ctx, slog.LevelDebug) {
				method, path := requestFields(ctx)
				slog.Debug("websocket connected", "method", method, "path", path)
			}
			return ctx, nil, nil
		},
		CloseFunc: func(ctx context.Context, code int) {
			if slog.Default().Enabled(ctx, slog.LevelDebug) {
				_, path := requestFields(ctx)
				slog.Debug("websocket disconnected", "path", path, "code", code)
			}
		},
		ErrorFunc: func(_ context.Context, err error) {
			slog.Warn("websocket", "err", err)
		},
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New(256))
	srv.Use(extension.Introspection{})
	logGraphQL(srv)
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
