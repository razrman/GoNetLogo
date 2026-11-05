package graph

import (
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
)

func TestProceduresReportCompiledSetup(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	var created struct {
		Session struct {
			Procedures []string
		}
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { procedures } }`, &created, client.Var("id", s.ID))
	if !containsProcedure(created.Session.Procedures, "setup") {
		t.Fatalf("fire procedures %v", created.Session.Procedures)
	}

	var missing struct {
		LoadModelSource struct {
			Procedures []string
		}
	}
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { procedures }
	}`, &missing, client.Var("id", s.ID), client.Var("name", "life.nlogo"),
		client.Var("src", "to setup-normal\nend\nto setup-random\nend\n"))
	if containsProcedure(missing.LoadModelSource.Procedures, "setup") {
		t.Fatalf("procedures %v", missing.LoadModelSource.Procedures)
	}
	if !containsProcedure(missing.LoadModelSource.Procedures, "setup-normal") ||
		!containsProcedure(missing.LoadModelSource.Procedures, "setup-random") {
		t.Fatalf("procedures %v", missing.LoadModelSource.Procedures)
	}
	err := c.Post(`mutation ($id: ID!) { setup(sessionId: $id) { id } }`, &struct {
		Setup struct{ ID string }
	}{}, client.Var("id", s.ID))
	if err == nil || !strings.Contains(err.Error(), "nothing named setup") {
		t.Fatalf("missing setup: %v", err)
	}

	var broken struct {
		LoadModelSource struct {
			Procedures []string
		}
	}
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { procedures }
	}`, &broken, client.Var("id", s.ID), client.Var("name", "broken.nlogo"),
		client.Var("src", "to Setup\n  error \"boom\"\nend\n"))
	if !containsProcedure(broken.LoadModelSource.Procedures, "setup") {
		t.Fatalf("procedures %v, want setup from to Setup", broken.LoadModelSource.Procedures)
	}
	err = c.Post(`mutation ($id: ID!) { setup(sessionId: $id) { id } }`, &struct {
		Setup struct{ ID string }
	}{}, client.Var("id", s.ID))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("defined setup: %v", err)
	}
}

func containsProcedure(names []string, want string) bool {
	want = strings.ToLower(want)
	for _, name := range names {
		if strings.EqualFold(name, want) {
			return true
		}
	}
	return false
}
