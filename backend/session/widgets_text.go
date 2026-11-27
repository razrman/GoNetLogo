package session

import (
	"fmt"
	"log/slog"
	"os"

	"gonetlogo/pkg/nl"
)

// SetWidgets replaces the interface from the Widgets tab. A parse error
// leaves the running widgets in place. Code, info, and shapes are not
// rewritten. Turtles and patches stay when the view bounds and wrapping
// do not change. New interface globals take the widget default. A global
// that the new interface no longer defines is dropped. Plots are built
// again from the text.
func (s *Session) SetWidgets(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("set widgets"); err != nil {
		return err
	}
	if s.model == nil || s.vm == nil || s.vm.World == nil {
		return fmt.Errorf("no model is loaded")
	}
	if s.modelSource == "" {
		return fmt.Errorf("this model has no stored text to update")
	}
	section := nl.ExtractWidgetSection(s.modelSource)
	view, widgets, notes, err := nl.ParseWidgetSection(text, section.XML, s.model.Is3D)
	if err != nil {
		return err
	}
	if view.Is3D != s.model.Is3D {
		return fmt.Errorf("the widgets tab cannot switch between a 2D and a 3D world")
	}
	if err := validateWidgetView(s.vm.World, view); err != nil {
		return err
	}
	next, err := nl.ReplaceWidgetSection(s.modelSource, text)
	if err != nil {
		return err
	}
	return s.installWidgetsLocked(view, widgets, notes, next)
}

// SetWidgetYAML replaces the interface widgets from the Widgets tab list.
// A bad document leaves the model unchanged. The returned text is the
// canonical list after a successful edit.
func (s *Session) SetWidgetYAML(text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("set widgets"); err != nil {
		return "", err
	}
	if s.model == nil || s.vm == nil || s.vm.World == nil {
		return "", fmt.Errorf("no model is loaded")
	}
	updated, from, err := nl.ApplyWidgetsYAML(s.model.Widgets, text)
	if err != nil {
		return "", err
	}
	next, err := nl.WriteWidgets(s.modelSource, updated, from)
	if err != nil {
		return "", err
	}
	section := nl.ExtractWidgetSection(next)
	view, widgets, notes, err := nl.ParseWidgetSection(section.Text, section.XML, s.model.Is3D)
	if err != nil {
		return "", err
	}
	if err := validateWidgetView(s.vm.World, view); err != nil {
		return "", err
	}
	if err := s.installWidgetsLocked(view, widgets, notes, next); err != nil {
		return "", err
	}
	return nl.FormatWidgetsYAML(s.model.Widgets)
}

// WidgetYAML is the Widgets tab document for every widget, in model order.
func (s *Session) WidgetYAML() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return "", fmt.Errorf("no model is loaded")
	}
	return nl.FormatWidgetsYAML(s.model.Widgets)
}

func (s *Session) installWidgetsLocked(view nl.ViewSpec, widgets []nl.Widget, notes []string, next string) error {
	oldKind := map[string]string{}
	for _, w := range s.model.Widgets {
		if w.Variable != "" {
			oldKind[w.Variable] = w.Kind
		}
	}
	keep := map[string]bool{}
	for _, w := range widgets {
		if w.Variable != "" {
			keep[w.Variable] = true
		}
	}

	world := s.vm.World
	resize := !widgetGeometrySame(world, view)
	s.model.View = view
	s.model.Widgets = widgets
	s.model.Notes = notes
	s.modelSource = next
	if s.vm.Prog != nil {
		s.vm.Prog.InterfaceVars = keep
		s.vm.Prog.Recompile()
	}
	s.vm.DoLocked(func() {
		for name := range oldKind {
			if !keep[name] {
				delete(world.Globals(), name)
			}
		}
		if s.vm.Prog != nil {
			world.SetProgramVars(s.vm.Prog)
		}
		for _, widget := range widgets {
			if widget.Variable == "" || oldKind[widget.Variable] == widget.Kind {
				continue
			}
			if v, ok := widgetDefault(widget); ok {
				world.Globals()[widget.Variable] = v
			}
		}
		s.vm.SetHasOutput(s.model.HasOutput())
		s.vm.InstallPlots(nl.PlotSpecs(widgets))
		s.vm.UpdatesOnTicks = view.UpdateMode == nl.UpdateOnTicks
		if !resize {
			if view.PatchSize > 0 {
				world.PatchSize = view.PatchSize
			}
			if view.FontSize > 0 {
				world.LabelFont = view.FontSize
			}
		}
	})
	if resize {
		var resizeErr error
		if world.Is3D() {
			resizeErr = world.ApplyView3D(view.MinPxcor, view.MaxPxcor, view.MinPycor, view.MaxPycor, view.MinPzcor, view.MaxPzcor, view.PatchSize, view.WrapX, view.WrapY, view.WrapZ)
		} else {
			resizeErr = world.ApplyView(view.MinPxcor, view.MaxPxcor, view.MinPycor, view.MaxPycor, view.PatchSize, view.WrapX, view.WrapY)
		}
		if resizeErr != nil {
			return resizeErr
		}
		s.frozenWorld.Store(nil)
	}
	s.compileButtonsLocked()
	if s.uploadPath != "" {
		if err := os.WriteFile(s.uploadPath, []byte(next), 0o600); err != nil {
			slog.Warn("session storage", "session", s.id, "err", err)
		}
	}
	s.broadcastLocked()
	return nil
}

// WidgetText is the interface section of the loaded model.
func (s *Session) WidgetText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nl.ExtractWidgetSection(s.modelSource).Text
}

func widgetGeometrySame(w *nl.World, v nl.ViewSpec) bool {
	if w.MinPxcor != v.MinPxcor || w.MaxPxcor != v.MaxPxcor || w.MinPycor != v.MinPycor || w.MaxPycor != v.MaxPycor {
		return false
	}
	if w.WrapX != v.WrapX || w.WrapY != v.WrapY {
		return false
	}
	if w.Is3D() && (w.MinPzcor != v.MinPzcor || w.MaxPzcor != v.MaxPzcor || w.WrapZ != v.WrapZ) {
		return false
	}
	return true
}

func validateWidgetView(w *nl.World, v nl.ViewSpec) error {
	if v.PatchSize <= 0 || v.PatchSize > 100 {
		return fmt.Errorf("patch size must be positive and at most 100")
	}
	if w.Is3D() {
		return nl.ValidWorld3D(v.MinPxcor, v.MaxPxcor, v.MinPycor, v.MaxPycor, v.MinPzcor, v.MaxPzcor)
	}
	return nl.ValidWorld(v.MinPxcor, v.MaxPxcor, v.MinPycor, v.MaxPycor)
}

func widgetDefault(w nl.Widget) (nl.Value, bool) {
	switch w.Kind {
	case "slider":
		return nl.Num(w.Value), true
	case "switch":
		return nl.Bool(w.On), true
	case "chooser":
		if w.Current >= 0 && w.Current < len(w.Choices) {
			return w.Choices[w.Current], true
		}
	case "input":
		if w.Text != "" {
			return nl.Str(w.Text), true
		}
		return nl.Num(w.Value), true
	}
	return nl.Value{}, false
}
