package main

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type httpRequestKey struct{}

func withHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

func httpRequest(ctx context.Context) *http.Request {
	r, _ := ctx.Value(httpRequestKey{}).(*http.Request)
	return r
}

func requestFields(ctx context.Context) (method, path string) {
	if r := httpRequest(ctx); r != nil {
		return r.Method, r.URL.Path
	}
	return "", ""
}

// logHTTP records non-GraphQL requests at debug. /graphql is logged once
// per operation by the GraphQL handler, including the operation name.
// The request is always stored on the context so websocket hooks can see it.
func logHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(withHTTPRequest(r.Context(), r))
		logger := slog.Default()
		if !logger.Enabled(r.Context(), slog.LevelDebug) || r.URL.Path == "/graphql" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logGraphQL records one debug line per query or mutation. Subscription
// payloads are not logged; connect and disconnect are logged elsewhere.
// A response that carries errors is a warning, including on a subscription.
func logGraphQL(srv *handler.Server) {
	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		if !graphql.HasOperationContext(ctx) {
			return next(ctx)
		}
		oc := graphql.GetOperationContext(ctx)
		kind, name := opLabel(oc)
		method, path := requestFields(ctx)
		sessionID := shortSessionID(oc.Variables)
		start := time.Now()
		sub := kind == string(ast.Subscription)
		logger := slog.Default()
		if sub && logger.Enabled(ctx, slog.LevelDebug) {
			logger.Debug("graphql subscription", fields(method, path, name, "", sessionID, 0, "")...)
		}
		inner := next(ctx)
		return func(ctx context.Context) *graphql.Response {
			resp := inner(ctx)
			if resp != nil && len(resp.Errors) > 0 {
				slog.Warn("graphql request", fields(method, path, name, kind, sessionID, 0, joinGraphQLErrors(resp.Errors))...)
				return resp
			}
			if sub || !logger.Enabled(ctx, slog.LevelDebug) {
				return resp
			}
			logger.Debug("graphql request", fields(method, path, name, kind, sessionID, time.Since(start), "")...)
			return resp
		}
	})
}

func fields(method, path, name, kind, sessionID string, duration time.Duration, errText string) []any {
	out := []any{"method", method, "path", path, "op", name}
	if kind != "" {
		out = append(out, "opType", kind)
	}
	if sessionID != "" {
		out = append(out, "session", sessionID)
	}
	if duration > 0 {
		out = append(out, "duration", duration)
	}
	if errText != "" {
		out = append(out, "err", errText)
	}
	return out
}

func opLabel(oc *graphql.OperationContext) (kind, name string) {
	if oc == nil {
		return "", ""
	}
	if oc.Operation != nil {
		kind = string(oc.Operation.Operation)
		if oc.OperationName == "" {
			for _, sel := range oc.Operation.SelectionSet {
				field, ok := sel.(*ast.Field)
				if ok {
					name = field.Name
					break
				}
			}
		}
	}
	if name == "" {
		name = oc.OperationName
	}
	return kind, name
}

func shortSessionID(vars map[string]any) string {
	if vars == nil {
		return ""
	}
	for _, key := range []string{"sessionId", "id"} {
		s, ok := vars[key].(string)
		if !ok || s == "" || len(s) > 64 || strings.ContainsAny(s, " \n\r") {
			continue
		}
		return s
	}
	return ""
}

func joinGraphQLErrors(errs gqlerror.List) string {
	var b strings.Builder
	n := 0
	for _, e := range errs {
		if e == nil || e.Message == "" {
			continue
		}
		if n == 5 {
			b.WriteString("; ...")
			break
		}
		if n > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Message)
		n++
	}
	return b.String()
}
