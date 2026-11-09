package session

import (
	"fmt"
	"log/slog"

	"gonetlogo/backend/graph/model"
	"gonetlogo/pkg/nl"
)

// SetDynamics replaces the diagram from the Dynamics tab text and
// recompiles. A definition that does not parse leaves the model alone.
// A diagram that parses but does not compile stays on the model, and the
// previous world keeps running, matching SetCode. Caller must not be
// running go.
func (s *Session) SetDynamics(definition string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("set dynamics"); err != nil {
		return err
	}
	if s.model == nil {
		return fmt.Errorf("no model is loaded")
	}
	sd, err := nl.ParseDynamicsDefinition(definition)
	if err != nil {
		return err
	}
	s.model.Dynamics = sd
	vm, err := s.model.NewVM(0)
	if err != nil {
		explained := nl.ExplainDynamicsCompile(s.model.Code, sd.Source(), err)
		slog.Warn("model compile failed", "session", s.id, "model", s.modelName, "bytes", len(definition), "err", explained)
		s.broadcastLocked()
		return explained
	}
	s.installLocked(s.modelName, s.model, vm)
	s.broadcastLocked()
	return nil
}

func dynamicsView(sd *nl.SystemDynamics) *model.SystemDynamics {
	if sd == nil {
		return nil
	}
	stocks := make([]model.DynamicsStock, len(sd.Stocks))
	for i, stock := range sd.Stocks {
		stocks[i] = model.DynamicsStock{
			Name:          stock.Name,
			Initial:       stock.Initial,
			AllowNegative: stock.AllowNegative,
		}
	}
	flows := make([]model.DynamicsFlow, len(sd.Rates))
	for i, flow := range sd.Rates {
		flows[i] = model.DynamicsFlow{
			Name:       flow.Name,
			Expression: flow.Expression,
			Source:     flow.Source,
			Sink:       flow.Sink,
		}
	}
	converters := make([]model.DynamicsConverter, len(sd.Converters))
	for i, conv := range sd.Converters {
		converters[i] = model.DynamicsConverter{
			Name:       conv.Name,
			Expression: conv.Expression,
			Constant:   conv.Constant,
		}
	}
	return &model.SystemDynamics{
		Dt:         sd.DT,
		Stocks:     stocks,
		Flows:      flows,
		Converters: converters,
		Definition: sd.Definition(),
		Source:     sd.Source(),
	}
}
