package graph

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"gonetlogo/pkg/nl"
)

const dynamicsQuery = `
  query ($id: ID!) {
    session(id: $id) {
      dynamics {
        dt
        definition
        source
        stocks { name initial allowNegative }
        flows { name expression source sink }
        converters { name expression constant }
      }
    }
  }
`

type apiDynamics struct {
	Dt         string
	Definition string
	Source     string
	Stocks     []struct {
		Name          string
		Initial       string
		AllowNegative bool
	}
	Flows []struct {
		Name       string
		Expression string
		Source     string
		Sink       string
	}
	Converters []struct {
		Name       string
		Expression string
		Constant   bool
	}
}

func TestPlainModelHasNoDynamics(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	if got := sessionDynamics(t, c, s.ID); got != nil {
		t.Fatalf("fire exposed a diagram: %+v", got)
	}
}

func TestSystemDynamicsLibraryExposure(t *testing.T) {
	t.Setenv("MODELS_DIR", filepath.Join("..", "..", "models"))
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	cases := []struct {
		path  string
		check func(t *testing.T, got *apiDynamics, sd *nl.SystemDynamics)
	}{
		{
			path: "library/Sample Models/System Dynamics/Exponential Growth.nlogox",
			check: func(t *testing.T, got *apiDynamics, sd *nl.SystemDynamics) {
				if got.Dt != "0.001" || len(got.Stocks) != 1 || got.Stocks[0].Name != "stock" || got.Stocks[0].Initial != "1" || !got.Stocks[0].AllowNegative {
					t.Fatalf("stock %+v dt %q", got.Stocks, got.Dt)
				}
				if len(got.Flows) != 1 || got.Flows[0].Name != "inflow" || got.Flows[0].Expression != "stock * growth-rate" || got.Flows[0].Source != "" || got.Flows[0].Sink != "stock" {
					t.Fatalf("inflow %+v", got.Flows)
				}
				if len(got.Converters) != 1 || got.Converters[0].Name != "growth-rate" || got.Converters[0].Expression != "growth-rate-slider" || got.Converters[0].Constant {
					t.Fatalf("growth-rate %+v", got.Converters)
				}
				for _, proc := range []string{"to system-dynamics-setup", "to system-dynamics-go", "to system-dynamics-do-plot"} {
					if !strings.Contains(got.Source, proc) {
						t.Fatalf("source missing %s", proc)
					}
				}
				if got.Source != sd.Source() {
					t.Fatal("source is not what Compile appends")
				}
			},
		},
		{
			path: "library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox",
			check: func(t *testing.T, got *apiDynamics, sd *nl.SystemDynamics) {
				sheep, wolves := stockNamed(t, got, "sheep"), stockNamed(t, got, "wolves")
				if sheep.Initial != "100" || sheep.AllowNegative || wolves.Initial != "30" || wolves.AllowNegative {
					t.Fatalf("sheep %+v wolves %+v", sheep, wolves)
				}
				births, deaths := flowNamed(t, got, "sheep-births"), flowNamed(t, got, "sheep-deaths")
				if births.Source != "" || births.Sink != "sheep" || births.Expression != "sheep-birth-rate * sheep" {
					t.Fatalf("sheep-births %+v", births)
				}
				if deaths.Source != "sheep" || deaths.Sink != "" {
					t.Fatalf("sheep-deaths %+v", deaths)
				}
				rate := converterNamed(t, got, "sheep-birth-rate")
				if rate.Expression != ".04" || !rate.Constant {
					t.Fatalf("sheep-birth-rate %+v", rate)
				}
				if got.Source != sd.Source() {
					t.Fatal("source is not what Compile appends")
				}
			},
		},
		{
			path: "library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo.nlogox",
			check: func(t *testing.T, got *apiDynamics, sd *nl.SystemDynamics) {
				gaps := stockNamed(t, got, "gaps")
				if gaps.AllowNegative || !strings.Contains(gaps.Initial, "0.6") || !strings.Contains(gaps.Initial, ";; since we don't know what order") {
					t.Fatalf("gaps %+v", gaps)
				}
				if got.Dt != "1.0" {
					t.Fatalf("dt %q", got.Dt)
				}
				if got.Source != sd.Source() {
					t.Fatal("source is not what Compile appends")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			m, err := nl.LoadFile(filepath.Join("..", "..", "models", tc.path))
			if err != nil {
				t.Fatal(err)
			}
			var loaded struct {
				LoadModelFile struct{ ModelName string }
			}
			c.MustPost(`mutation ($id: ID!, $path: String!) {
				loadModelFile(sessionId: $id, path: $path) { modelName }
			}`, &loaded, client.Var("id", s.ID), client.Var("path", tc.path))
			got := sessionDynamics(t, c, s.ID)
			if got == nil {
				t.Fatal("library model exposed no diagram")
			}
			if _, err := nl.ParseDynamicsDefinition(got.Definition); err != nil {
				t.Fatalf("definition does not parse: %v", err)
			}
			assertAPIDynamics(t, m.Dynamics, got)
			tc.check(t, got, m.Dynamics)
		})
	}
}

func TestSetDynamicsAppliesAndReportsErrors(t *testing.T) {
	t.Setenv("MODELS_DIR", filepath.Join("..", "..", "models"))
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	path := "library/Sample Models/System Dynamics/Exponential Growth.nlogox"
	var loaded struct {
		LoadModelFile struct{ ModelName string }
	}
	c.MustPost(`mutation ($id: ID!, $path: String!) {
		loadModelFile(sessionId: $id, path: $path) { modelName }
	}`, &loaded, client.Var("id", s.ID), client.Var("path", path))
	before := sessionDynamics(t, c, s.ID)
	if before == nil {
		t.Fatal("missing diagram")
	}
	setup(t, c, s.ID)
	if got := reporter(t, c, s.ID, "stock"); got != "1" {
		t.Fatalf("stock after setup %q", got)
	}

	badParse := applyDynamics(t, c, s.ID, "this is not a diagram\n")
	if badParse.Ok || badParse.Error == nil || !strings.Contains(*badParse.Error, "line 1:") {
		t.Fatalf("parse error %+v", badParse)
	}
	if got := sessionDynamics(t, c, s.ID); got == nil || got.Dt != before.Dt || got.Definition != before.Definition {
		t.Fatal("a parse error changed the diagram")
	}
	if got := reporter(t, c, s.ID, "stock"); got != "1" {
		t.Fatalf("stock after parse error %q", got)
	}

	badCompile := strings.Replace(before.Definition, "expression stock * growth-rate\n", "expression (\n", 1)
	if badCompile == before.Definition {
		t.Fatal("could not break the flow expression")
	}
	compiled := applyDynamics(t, c, s.ID, badCompile)
	if compiled.Ok || compiled.Error == nil || !strings.Contains(*compiled.Error, "generated procedures, line") || !strings.Contains(*compiled.Error, " | ") {
		t.Fatalf("compile error %+v", compiled)
	}
	if got := reporter(t, c, s.ID, "stock"); got != "1" {
		t.Fatalf("failed compile replaced the world, stock %q", got)
	}

	edited := strings.Replace(before.Definition, "initial 1\n", "initial 4\n", 1)
	if edited == before.Definition {
		t.Fatal("initial 1 was not in the definition")
	}
	okRes := applyDynamics(t, c, s.ID, edited)
	if !okRes.Ok || okRes.Error != nil {
		t.Fatalf("apply %+v", okRes)
	}
	if okRes.Session.Dynamics == nil || !strings.Contains(okRes.Session.Dynamics.Source, "set stock 4") {
		t.Fatalf("applied source missing the new initial: %+v", okRes.Session.Dynamics)
	}
	found := false
	for _, name := range okRes.Session.Procedures {
		if name == "system-dynamics-setup" {
			found = true
		}
	}
	if !found {
		t.Fatalf("procedures %v", okRes.Session.Procedures)
	}
	setup(t, c, s.ID)
	if got := reporter(t, c, s.ID, "stock"); got != "4" {
		t.Fatalf("stock after apply %q", got)
	}

	var goResp struct{ Go struct{ ID string } }
	c.MustPost(`mutation ($id: ID!) { go(sessionId: $id) { id } }`, &goResp, client.Var("id", s.ID))
	locked := applyDynamics(t, c, s.ID, edited)
	if locked.Ok || locked.Error == nil || *locked.Error != "Stop the model before changing the system dynamics." {
		t.Fatalf("lock %+v", locked)
	}
	if got := sessionDynamics(t, c, s.ID); got == nil || !strings.Contains(got.Definition, "initial 4") {
		t.Fatal("a rejected edit changed the diagram")
	}
	var stopped struct{ Stop struct{ ID string } }
	c.MustPost(`mutation ($id: ID!) { stop(sessionId: $id) { id } }`, &stopped, client.Var("id", s.ID))
}

type dynamicsResult struct {
	Ok      bool
	Error   *string
	Session struct {
		Procedures []string
		Dynamics   *apiDynamics
	}
}

func applyDynamics(t *testing.T, c *client.Client, id, definition string) dynamicsResult {
	t.Helper()
	var res struct {
		SetDynamics dynamicsResult
	}
	c.MustPost(`mutation ($id: ID!, $definition: String!) {
		setDynamics(sessionId: $id, definition: $definition) {
		  ok
		  error
		  session {
		    procedures
		    dynamics {
		      dt definition source
		      stocks { name initial allowNegative }
		      flows { name expression source sink }
		      converters { name expression constant }
		    }
		  }
		}
	}`, &res, client.Var("id", id), client.Var("definition", definition))
	return res.SetDynamics
}

func sessionDynamics(t *testing.T, c *client.Client, id string) *apiDynamics {
	t.Helper()
	var res struct {
		Session struct{ Dynamics *apiDynamics }
	}
	c.MustPost(dynamicsQuery, &res, client.Var("id", id))
	return res.Session.Dynamics
}

func reporter(t *testing.T, c *client.Client, id, text string) string {
	t.Helper()
	var res struct {
		RunCommand struct {
			Ok     bool
			Output *string
			Error  *string
		}
	}
	c.MustPost(`mutation ($id: ID!, $text: String!) {
		runCommand(sessionId: $id, text: $text) { ok output error }
	}`, &res, client.Var("id", id), client.Var("text", text))
	if !res.RunCommand.Ok || res.RunCommand.Output == nil {
		errText := ""
		if res.RunCommand.Error != nil {
			errText = *res.RunCommand.Error
		}
		t.Fatalf("reporter %s: %s", text, errText)
	}
	return *res.RunCommand.Output
}

func assertAPIDynamics(t *testing.T, want *nl.SystemDynamics, got *apiDynamics) {
	t.Helper()
	if want == nil || got == nil {
		t.Fatalf("nil diagram")
	}
	if got.Dt != want.DT || got.Source != want.Source() || got.Definition != want.Definition() {
		t.Fatalf("dt/source/definition mismatch dt %q want %q", got.Dt, want.DT)
	}
	if len(got.Stocks) != len(want.Stocks) || len(got.Flows) != len(want.Rates) || len(got.Converters) != len(want.Converters) {
		t.Fatalf("sizes stocks %d/%d flows %d/%d converters %d/%d",
			len(got.Stocks), len(want.Stocks), len(got.Flows), len(want.Rates), len(got.Converters), len(want.Converters))
	}
	for i, stock := range want.Stocks {
		g := got.Stocks[i]
		if g.Name != stock.Name || g.Initial != stock.Initial || g.AllowNegative != stock.AllowNegative {
			t.Fatalf("stock %d %+v, want %+v", i, g, stock)
		}
	}
	for i, flow := range want.Rates {
		g := got.Flows[i]
		if g.Name != flow.Name || g.Expression != flow.Expression || g.Source != flow.Source || g.Sink != flow.Sink {
			t.Fatalf("flow %d %+v, want %+v", i, g, flow)
		}
	}
	for i, conv := range want.Converters {
		g := got.Converters[i]
		if g.Name != conv.Name || g.Expression != conv.Expression || g.Constant != conv.Constant {
			t.Fatalf("converter %d %+v, want %+v", i, g, conv)
		}
	}
}

func stockNamed(t *testing.T, got *apiDynamics, name string) struct {
	Name          string
	Initial       string
	AllowNegative bool
} {
	t.Helper()
	for _, stock := range got.Stocks {
		if stock.Name == name {
			return stock
		}
	}
	t.Fatalf("no stock %s", name)
	return got.Stocks[0]
}

func flowNamed(t *testing.T, got *apiDynamics, name string) struct {
	Name, Expression, Source, Sink string
} {
	t.Helper()
	for _, flow := range got.Flows {
		if flow.Name == name {
			return flow
		}
	}
	t.Fatalf("no flow %s", name)
	return got.Flows[0]
}

func converterNamed(t *testing.T, got *apiDynamics, name string) struct {
	Name, Expression string
	Constant         bool
} {
	t.Helper()
	for _, conv := range got.Converters {
		if conv.Name == name {
			return conv
		}
	}
	t.Fatalf("no converter %s", name)
	return got.Converters[0]
}
