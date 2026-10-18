package gui

import (
	"image/png"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"gonetlogo/models"
	"gonetlogo/pkg/nl"
)

func TestMaxFPSEntryClamp(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	w.SetContent(s.layout)
	w.Resize(fyne.NewSize(1200, 800))
	if s.maxFPS.Text != "24" || s.frameRate != nl.DefaultFrameRate {
		t.Fatalf("default text %q rate %d", s.maxFPS.Text, s.frameRate)
	}
	s.maxFPS.SetText("500")
	if s.maxFPS.Text != "120" || s.frameRate != 120 {
		t.Fatalf("high text %q rate %d", s.maxFPS.Text, s.frameRate)
	}
	s.maxFPS.SetText("0")
	if s.maxFPS.Text != "1" || s.frameRate != 1 {
		t.Fatalf("low text %q rate %d", s.maxFPS.Text, s.frameRate)
	}
	s.maxFPS.SetText("24")
	if s.maxFPS.Size().Width < 70 {
		t.Fatalf("max fps box width %v, want room for 120", s.maxFPS.Size().Width)
	}
}

func TestDesktopWolfSheepFrameCap(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("GoNetLogo")
	s := newSession(models.FS, w)
	w.SetContent(s.layout)
	w.Resize(fyne.NewSize(1200, 800))
	raw, err := fs.ReadFile(models.FS, "wolfsheep.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.load("wolfsheep.nlogo", string(raw))
	if s.vm == nil || s.vm.World.Width() != 101 || s.vm.World.Height() != 101 || s.vm.World.PatchSize != 5 {
		t.Fatalf("world %v patch %v", s.vm.World, s.vm.World.PatchSize)
	}
	goIdx := -1
	for i, c := range s.ctls {
		if strings.EqualFold(c.widget.Display, "go") {
			goIdx = i
		}
	}
	if goIdx < 0 {
		t.Fatal("no go button")
	}

	sample := func(fps int) (dropped int, published float64, ticks, tps string) {
		t.Helper()
		s.pressNamed("setup")
		s.mu.Lock()
		if s.pub.Dropped() != 0 {
			s.mu.Unlock()
			t.Fatalf("setup left %d drops", s.pub.Dropped())
		}
		s.running = goIdx
		s.speed.Value = 100
		s.frameRate = fps
		s.mu.Unlock()
		end := time.Now().Add(2 * time.Second)
		for time.Now().Before(end) {
			s.tick()
		}
		s.mu.Lock()
		dropped = s.pub.Dropped()
		published = s.pub.PublishedFPS()
		ticks = s.ticks.Text
		tps = s.tps.Text
		s.running = -1
		s.refreshRunLabels()
		s.mu.Unlock()
		t.Logf("cap %d %s %s drop-label will follow dropped %d publishedFps %.2f", fps, tps, ticks, dropped, published)
		if published > float64(fps)+0.05 {
			t.Fatalf("published fps %.2f exceeds cap %d", published, fps)
		}
		return
	}
	d24, p24, ticks24, tps24 := sample(24)
	s.maxFPS.SetText("120")
	s.mu.Lock()
	s.refreshRunLabels()
	s.mu.Unlock()
	if img := w.Canvas().Capture(); img != nil {
		if f, err := os.Create("/tmp/desktop-framecap.png"); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}
	d60, p60, ticks60, tps60 := sample(60)
	t.Logf("desktop wolf sheep cap 24: %s %s dropped %d published %.2f", tps24, ticks24, d24, p24)
	t.Logf("desktop wolf sheep cap 60: %s %s dropped %d published %.2f", tps60, ticks60, d60, p60)
	if d24 <= d60 {
		t.Fatalf("expected more drops at 24 (%d) than 60 (%d)", d24, d60)
	}
}
