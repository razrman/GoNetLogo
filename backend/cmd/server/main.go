// Command server is the GraphQL API for GoNetLogo.
// It owns the interpreter, the world, and one session per client.
// Built with -tags web, it also serves the React UI on the same port.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
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
	"gonetlogo/models"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:43131", "listen address. "+envAddr+" or "+envPort+" is used when this flag is omitted; an explicit flag wins. "+envPort+" replaces the port of this default")
	webDir := flag.String("web", "", "directory of a built frontend (overrides files embedded with -tags web)")
	logs := logcfg.Register(flag.CommandLine)
	pprofOn := flag.Bool("pprof", false, "serve Go pprof handlers on 127.0.0.1:6060 only; off by default")
	sessionGrace := flag.String("session-grace", "", "how long a disconnected web session stays alive so a reload can reconnect (default 15s, or "+session.EnvGrace+")")
	sessionIdle := flag.String("session-idle", "", "remove a detached web session that has been quiet for this long (default 5m, or "+session.EnvIdle+")")
	sessionSweep := flag.String("session-sweep", "", "how often to look for idle web sessions (default 30s, or "+session.EnvSweep+")")
	sessionHeartbeat := flag.String("session-heartbeat", "", "remove a detached web session with no heartbeat or websocket activity for this long (default 45s, three 15s beats, or "+session.EnvHeartbeat+")")
	shutdownFor := flag.String("shutdown-timeout", "", "how long to drain connections after SIGINT or SIGTERM (default 20s, or "+envShutdownTimeout+"); an explicit flag wins")
	maxSessions := flag.Int("max-sessions", 0, "maximum concurrent sessions; 0 means unlimited. "+envMaxSessions+" is used when this flag is omitted; an explicit flag wins")
	production := flag.Bool("production", false, "disable the playground and introspection, and replace the wildcard CORS header with -allowed-origins. "+envProduction+" is used when this flag is omitted; an explicit flag wins")
	allowedOrigins := flag.String("allowed-origins", "", "comma-separated http(s) origins allowed in production mode; same-origin is always allowed. "+envAllowedOrigins+" is used when this flag is omitted; an explicit flag wins")
	flag.Parse()
	if _, err := logs.Apply(os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "gonetlogo-server: %v\n", err)
		os.Exit(2)
	}
	defer logcfg.Close()
	defer logcfg.Recover()
	logcfg.CloseOnSignal()
	timeout, err := resolveShutdownTimeout(*shutdownFor, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "shutdown timeout", "err", err)
	}
	prod, err := resolveProduction(flagExplicit("production"), *production, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "production", "err", err)
	}
	origins, err := resolveAllowedOrigins(flagExplicit("allowed-origins"), *allowedOrigins, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "allowed origins", "err", err)
	}
	pol := newOriginPolicy(prod, origins)
	listen, err := resolveAddr(flagExplicit("addr"), *addr, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "listen address", "err", err)
	}

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

	life, err := session.ResolveLifecycle(*sessionGrace, *sessionIdle, *sessionSweep, *sessionHeartbeat, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "session lifecycle", "err", err)
	}
	slog.Info("session lifecycle", "grace", life.Grace, "idle", life.Idle, "sweep", life.Sweep, "heartbeat", life.Heartbeat)
	capN, err := resolveMaxSessions(flagExplicit("max-sessions"), *maxSessions, os.Getenv)
	if err != nil {
		logcfg.Fail(2, "max sessions", "err", err)
	}
	store := session.NewStoreWith(life)
	store.SetMaxSessions(capN)
	defer store.Close()
	slog.Info("session cap", "maxSessions", capN)
	if dir, err := session.ModelsDir(); err != nil {
		slog.Warn("model files unavailable; embedded demos still load", "err", err)
	} else {
		slog.Info("model files confined", "dir", dir)
	}
	if dir, ok := models.OpenDir(); ok {
		slog.Info("model open folder", "dir", dir)
	} else {
		slog.Warn("model open folder not found; the web Open button uses a local file chooser")
	}
	srv := newHandler(store, pol)

	ui, err := openUI(*webDir)
	if err != nil {
		logcfg.Fail(1, "web UI", "err", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/graphql", srv)
	mux.Handle("/session/end", session.EndHandler(store))
	mux.Handle("/session/heartbeat", session.HeartbeatHandler(store))
	mux.Handle("/model-asset/", modelAssetHandler())
	mountUI(mux, ui, pol.production)
	if ui != nil {
		slog.Info("web UI", "url", "http://"+listen+"/")
	}
	if pol.production {
		slog.Info("production", "playground", false, "introspection", false, "allowedOrigins", origins)
	} else if ui != nil {
		slog.Info("GraphQL playground", "url", "http://"+listen+"/playground")
	} else {
		slog.Info("GraphQL playground", "url", "http://"+listen+"/")
	}
	slog.Info("GraphQL endpoint", "url", "http://"+listen+"/graphql")
	slog.Info("health", "url", "http://"+listen+"/healthz")
	slog.Info("shutdown timeout", "timeout", timeout.String())
	httpSrv := &http.Server{
		Addr:    listen,
		Handler: rootHandler(logHTTP(withCORS(pol, mux))),
	}
	// The hook runs on the signal goroutine, before CloseOnSignal flushes
	// the log. It must close the store itself: that flush then exits, and
	// main's defers do not run.
	logcfg.OnSignal(func(sig os.Signal) {
		slog.Info("shutdown", "signal", sig.String(), "timeout", timeout.String())
		if err := shutdownHTTP(httpSrv, timeout, store.Close); err != nil {
			slog.Error("shutdown", "err", err)
		}
	})
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		store.Close()
		logcfg.Fail(1, "listen", "addr", listen, "err", err)
	}
	// Shutdown unblocks ListenAndServe before the hook closes the store.
	// Stay here so these defers do not race the hook's exit.
	select {}
}

func newHandler(store *session.Store, pol originPolicy) http.Handler {
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{Store: store}}))
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: pol.allows,
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
	if !pol.production {
		srv.Use(extension.Introspection{})
	}
	logGraphQL(srv)
	return srv
}

// mountUI registers the web app and, outside production mode, the GraphQL
// playground. Production mode answers /playground with 404 so a built UI
// cannot fall through to the playground, and it does not mount a
// playground at / when no UI is embedded.
func mountUI(mux *http.ServeMux, ui fs.FS, production bool) {
	if production {
		mux.HandleFunc("/playground", http.NotFound)
		if ui != nil {
			mux.Handle("/", spaHandler(ui))
		}
		return
	}
	if ui != nil {
		// The app owns /. The playground stays available beside it.
		mux.Handle("/", spaHandler(ui))
		mux.Handle("/playground", playground.Handler("GoNetLogo GraphQL", "/graphql"))
		return
	}
	mux.Handle("/", playground.Handler("GoNetLogo GraphQL", "/graphql"))
}
