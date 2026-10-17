// Package preview serves the modeling environment in a browser.
// The page drives the same interpreter and world renderer as the desktop app.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gonetlogo/internal/nl"
)

// Serve loads the built-in models and blocks while serving the preview.
func Serve(addr string, models fs.FS) error {
	names, err := modelNames(models)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no models to preview")
	}
	a := &app{models: models, names: names, running: -1, speed: 28}
	if err := a.useModel(names[0], true); err != nil {
		return err
	}
	go a.loop()

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/action", a.handleAction)
	mux.HandleFunc("/frame.png", a.handleFrame)
	srv := &http.Server{Addr: addr, Handler: mux}
	fmt.Printf("GoNetLogo preview at http://%s\n", addr)
	return srv.ListenAndServe()
}

type app struct {
	mu      sync.Mutex
	models  fs.FS
	names   []string
	model   *nl.FileModel
	vm      *nl.VM
	stmts   [][]nl.Stmt
	running int
	speed   float64
	lines   []string
	gen     int
	frame   int
}

func (a *app) loop() {
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		a.mu.Lock()
		a.frame++
		if a.running >= 0 && a.vm != nil {
			n := stepsFor(a.speed, a.frame)
			for i := 0; i < n; i++ {
				stopped, err := a.runIndex(a.running)
				if err != nil {
					a.log(err.Error())
					a.running = -1
					break
				}
				if stopped {
					a.running = -1
					break
				}
			}
		}
		a.mu.Unlock()
	}
}

func (a *app) useModel(name string, autogo bool) error {
	b, err := fs.ReadFile(a.models, name)
	if err != nil {
		return err
	}
	m, err := nl.ParseModel(name, string(b))
	if err != nil {
		return err
	}
	vm, err := m.NewVM(0)
	if err != nil {
		return err
	}
	a.model = m
	a.vm = vm
	a.vm.OnPrint = func(line string) { a.log(line) }
	a.stmts = make([][]nl.Stmt, len(m.Widgets))
	for i := range m.Widgets {
		w := m.Widgets[i]
		if w.Kind != "button" {
			continue
		}
		stmts, err := nl.CompileButton(vm.Prog, w.Source, w.Agent)
		if err != nil {
			a.log(w.Display + ": " + err.Error())
			continue
		}
		a.stmts[i] = stmts
	}
	a.running = -1
	a.gen++
	a.log("loaded " + name)
	if err := a.runNamed("setup"); err != nil {
		a.log("setup: " + err.Error())
	}
	if autogo {
		if i := a.foreverIndex("go"); i >= 0 {
			a.running = i
		}
	}
	return nil
}

func (a *app) foreverIndex(name string) int {
	for i, w := range a.model.Widgets {
		if w.Kind == "button" && w.Forever && (strings.EqualFold(w.Display, name) || strings.EqualFold(strings.TrimSpace(w.Source), name)) {
			return i
		}
	}
	return -1
}

func (a *app) runNamed(name string) error {
	for i, w := range a.model.Widgets {
		if w.Kind != "button" {
			continue
		}
		if strings.EqualFold(w.Display, name) || strings.EqualFold(strings.TrimSpace(w.Source), name) {
			_, err := a.runIndex(i)
			return err
		}
	}
	if a.vm.Prog.Proc(name) != nil {
		_, err := a.vm.Call(name)
		return err
	}
	return fmt.Errorf("no button or procedure named %s", name)
}

func (a *app) runIndex(i int) (bool, error) {
	if i < 0 || i >= len(a.stmts) {
		return false, fmt.Errorf("no such button")
	}
	return a.vm.RunCompiled(a.stmts[i])
}

func (a *app) log(line string) {
	a.lines = append(a.lines, line)
	if len(a.lines) > 80 {
		a.lines = a.lines[len(a.lines)-80:]
	}
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, page)
}

func (a *app) handleFrame(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vm == nil {
		http.Error(w, "no model", http.StatusServiceUnavailable)
		return
	}
	img := nl.RenderWorld(a.vm.World, 14)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.state())
}

type widgetJSON struct {
	Index    int      `json:"index"`
	Kind     string   `json:"kind"`
	Display  string   `json:"display"`
	Forever  bool     `json:"forever"`
	Disabled bool     `json:"disabled"`
	Min      float64  `json:"min,omitempty"`
	Max      float64  `json:"max,omitempty"`
	Value    float64  `json:"value,omitempty"`
	Step     float64  `json:"step,omitempty"`
	Units    string   `json:"units,omitempty"`
	On       bool     `json:"on,omitempty"`
	Reading  string   `json:"reading,omitempty"`
	Choices  []string `json:"choices,omitempty"`
	Current  int      `json:"current,omitempty"`
	Text     string   `json:"text,omitempty"`
}

func (a *app) state() map[string]any {
	ticks := ""
	started := false
	if a.vm != nil && a.vm.World.TicksStarted() {
		if n, err := a.vm.World.Ticks(); err == nil {
			ticks = strconv.FormatFloat(n, 'f', -1, 64)
			started = true
		}
	}
	var widgets []widgetJSON
	if a.model != nil {
		for i, wdg := range a.model.Widgets {
			j := widgetJSON{Index: i, Kind: wdg.Kind, Display: wdg.Display, Forever: wdg.Forever}
			switch wdg.Kind {
			case "button":
				j.Disabled = wdg.DisableUntilTicks && a.vm != nil && !a.vm.World.TicksStarted()
			case "slider":
				j.Min, j.Max, j.Value, j.Step, j.Units = wdg.Min, wdg.Max, wdg.Value, wdg.Step, wdg.Units
				if a.vm != nil {
					if v, ok := a.vm.World.Global(wdg.Variable); ok && v.K == nl.KindNum {
						j.Value = v.N
					}
				}
			case "switch":
				j.On = wdg.On
				if a.vm != nil {
					if v, ok := a.vm.World.Global(wdg.Variable); ok && v.K == nl.KindBool {
						j.On = v.B
					}
				}
			case "monitor":
				j.Reading = "N/A"
				if a.vm != nil && wdg.Source != "" {
					if out, err := a.vm.ExecuteLine(wdg.Source); err == nil {
						j.Reading = out
					}
				}
			case "chooser":
				for _, c := range wdg.Choices {
					j.Choices = append(j.Choices, c.Format(false))
				}
				j.Current = wdg.Current
			case "input":
				j.Text = wdg.Text
			case "note", "plot":
			default:
				continue
			}
			widgets = append(widgets, j)
		}
	}
	code := ""
	modelName := ""
	notes := ""
	if a.model != nil {
		code = a.model.Code
		modelName = a.model.Name
		notes = strings.Join(a.model.Notes, " · ")
	}
	logCopy := append([]string(nil), a.lines...)
	return map[string]any{
		"generation":   a.gen,
		"model":        modelName,
		"models":       a.names,
		"ticks":        ticks,
		"ticksStarted": started,
		"running":      a.running >= 0,
		"speed":        a.speed,
		"notes":        notes,
		"log":          logCopy,
		"widgets":      widgets,
		"code":         code,
	}
}

type action struct {
	Op    string  `json:"op"`
	Name  string  `json:"name"`
	Index int     `json:"index"`
	Value float64 `json:"value"`
	On    bool    `json:"on"`
	Text  string  `json:"text"`
	Code  string  `json:"code"`
}

func (a *app) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", http.StatusMethodNotAllowed)
		return
	}
	var act action
	if err := json.NewDecoder(r.Body).Decode(&act); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var err error
	switch act.Op {
	case "model":
		err = a.useModel(act.Name, false)
	case "setup":
		a.running = -1
		err = a.runNamed("setup")
	case "go":
		if a.running >= 0 {
			a.running = -1
		} else if i := a.foreverIndex("go"); i >= 0 {
			a.running = i
		} else {
			err = a.runNamed("go")
		}
	case "button":
		err = a.press(act.Index)
	case "slider":
		err = a.setSlider(act.Index, act.Value)
	case "switch":
		err = a.setSwitch(act.Index, act.On)
	case "chooser":
		err = a.setChooser(act.Index, int(act.Value))
	case "input":
		err = a.setInput(act.Index, act.Text)
	case "speed":
		a.speed = act.Value
	case "command":
		err = a.command(act.Text)
	case "apply":
		err = a.apply(act.Code)
	default:
		err = fmt.Errorf("unknown action %s", act.Op)
	}
	if err != nil {
		a.log(err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": err == nil, "error": errString(err)})
}

func (a *app) press(i int) error {
	if a.model == nil || i < 0 || i >= len(a.model.Widgets) {
		return fmt.Errorf("no such button")
	}
	w := a.model.Widgets[i]
	if w.Kind != "button" {
		return fmt.Errorf("not a button")
	}
	if w.Forever {
		if a.running == i {
			a.running = -1
			return nil
		}
		if w.DisableUntilTicks && !a.vm.World.TicksStarted() {
			return fmt.Errorf("%s is disabled until ticks start", w.Display)
		}
		a.running = i
		return nil
	}
	_, err := a.runIndex(i)
	return err
}

func (a *app) setSlider(i int, v float64) error {
	if a.model == nil || i < 0 || i >= len(a.model.Widgets) {
		return fmt.Errorf("no such slider")
	}
	w := &a.model.Widgets[i]
	if w.Kind != "slider" {
		return fmt.Errorf("not a slider")
	}
	w.Value = v
	a.vm.World.SetGlobal(w.Variable, nl.Num(v))
	return nil
}

func (a *app) setSwitch(i int, on bool) error {
	if a.model == nil || i < 0 || i >= len(a.model.Widgets) {
		return fmt.Errorf("no such switch")
	}
	w := &a.model.Widgets[i]
	if w.Kind != "switch" {
		return fmt.Errorf("not a switch")
	}
	w.On = on
	a.vm.World.SetGlobal(w.Variable, nl.Bool(on))
	return nil
}

func (a *app) setChooser(i, cur int) error {
	if a.model == nil || i < 0 || i >= len(a.model.Widgets) {
		return fmt.Errorf("no such chooser")
	}
	w := &a.model.Widgets[i]
	if w.Kind != "chooser" || cur < 0 || cur >= len(w.Choices) {
		return fmt.Errorf("not a chooser")
	}
	w.Current = cur
	a.vm.World.SetGlobal(w.Variable, w.Choices[cur])
	return nil
}

func (a *app) setInput(i int, text string) error {
	if a.model == nil || i < 0 || i >= len(a.model.Widgets) {
		return fmt.Errorf("no such input")
	}
	w := &a.model.Widgets[i]
	w.Text = text
	if strings.EqualFold(w.InputType, "number") || strings.EqualFold(w.InputType, "color") || w.InputType == "Number" || w.InputType == "Color" {
		var n float64
		fmt.Sscan(text, &n)
		w.Value = n
		a.vm.World.SetGlobal(w.Variable, nl.Num(n))
	} else {
		a.vm.World.SetGlobal(w.Variable, nl.Str(text))
	}
	return nil
}

func (a *app) command(text string) error {
	text = strings.TrimSpace(text)
	if text == "" || a.vm == nil {
		return nil
	}
	out, err := a.vm.ExecuteLine(text)
	if err != nil {
		return err
	}
	if out != "" {
		a.log(out)
	}
	return nil
}

func (a *app) apply(code string) error {
	if a.model == nil {
		return fmt.Errorf("no model")
	}
	a.model.Code = code
	vm, err := a.model.NewVM(0)
	if err != nil {
		return err
	}
	a.vm = vm
	a.vm.OnPrint = func(line string) { a.log(line) }
	a.stmts = make([][]nl.Stmt, len(a.model.Widgets))
	for i := range a.model.Widgets {
		w := a.model.Widgets[i]
		if w.Kind != "button" {
			continue
		}
		stmts, err := nl.CompileButton(vm.Prog, w.Source, w.Agent)
		if err != nil {
			return err
		}
		a.stmts[i] = stmts
	}
	a.running = -1
	a.gen++
	a.log("code applied — press setup")
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func modelNames(models fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(models, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".nlogo") || strings.HasSuffix(n, ".nlogox") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

func stepsFor(speed float64, frame int) int {
	if speed >= 50 {
		return 1 + int((speed-50)/10)
	}
	gap := 1 + int((50-speed)/5)
	if gap < 1 {
		gap = 1
	}
	if frame%gap == 0 {
		return 1
	}
	return 0
}

const page = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GoNetLogo</title>
<style>
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body { margin: 0; font: 14px/1.4 "Segoe UI", system-ui, sans-serif; background: #eceae4; color: #1c1c1c; }
  header { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; background: #205c3a; color: #fff; padding: 10px 14px; }
  header strong { font-size: 16px; margin-right: 8px; }
  header select, header button, header input { font: inherit; }
  button { border: 0; padding: 6px 12px; cursor: pointer; background: #e7e4dc; }
  button.setup { background: #2e7848; color: #fff; }
  button.go { background: #b44632; color: #fff; min-width: 64px; }
  button:disabled { opacity: 0.45; cursor: default; }
  .speed { display: flex; align-items: center; gap: 8px; }
  .speed input { width: 120px; }
  main { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 12px; padding: 12px; }
  #view { background: #141414; min-height: 320px; display: flex; align-items: center; justify-content: center; }
  #world { width: 100%; height: auto; max-height: 68vh; image-rendering: pixelated; object-fit: contain; }
  aside { background: #fff; padding: 12px; overflow: auto; max-height: 72vh; }
  aside h2 { margin: 0 0 8px; font-size: 13px; letter-spacing: 0.04em; text-transform: uppercase; color: #205c3a; }
  .widget { margin: 0 0 12px; }
  .widget label { display: block; font-size: 13px; margin-bottom: 2px; }
  .reading { font-size: 20px; font-variant-numeric: tabular-nums; }
  #notes { color: #555; font-size: 12px; }
  footer { padding: 0 12px 16px; }
  #cmd { display: flex; gap: 8px; }
  #cmd input { flex: 1; font: inherit; padding: 6px 8px; }
  #log { margin-top: 8px; background: #fff; min-height: 64px; max-height: 120px; overflow: auto; padding: 8px; white-space: pre-wrap; font-family: ui-monospace, monospace; font-size: 12px; }
  #code { width: 100%; min-height: 420px; font-family: ui-monospace, monospace; font-size: 12px; }
  .tabs { display: flex; gap: 6px; padding: 0 12px; }
  .tabs button { background: transparent; border-bottom: 2px solid transparent; }
  .tabs button.on { border-color: #205c3a; font-weight: 600; }
  .hidden { display: none; }
  @media (max-width: 800px) { main { grid-template-columns: 1fr; } aside { max-height: none; } }
</style>
</head>
<body>
<header>
  <strong>GoNetLogo</strong>
  <select id="model"></select>
  <button class="setup" id="setup" type="button">setup</button>
  <button class="go" id="go" type="button">go</button>
  <span class="speed">speed <input id="speed" type="range" min="0" max="100" value="28"></span>
  <span id="ticks">ticks</span>
</header>
<div class="tabs">
  <button type="button" id="tab-iface" class="on">Interface</button>
  <button type="button" id="tab-code">Code</button>
</div>
<main id="iface">
  <div id="view"><img id="world" alt="World view"></div>
  <aside>
    <h2>Interface</h2>
    <div id="widgets"></div>
    <p id="notes"></p>
  </aside>
</main>
<main id="codepane" class="hidden">
  <textarea id="code" spellcheck="false"></textarea>
  <p><button type="button" id="apply">Apply code</button></p>
</main>
<footer>
  <form id="cmd"><input id="line" placeholder="observer command or reporter" autocomplete="off"><button type="submit">Run</button></form>
  <div id="log"></div>
</footer>
<script>
let gen = -1;
const widgets = document.getElementById('widgets');
async function post(body) {
  await fetch('/api/action', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(body)});
}
function build(st) {
  const sel = document.getElementById('model');
  sel.innerHTML = '';
  for (const name of st.models) {
    const o = document.createElement('option');
    o.value = name; o.textContent = name;
    if (name === st.model) o.selected = true;
    sel.appendChild(o);
  }
  widgets.innerHTML = '';
  for (const w of st.widgets) {
    const box = document.createElement('div');
    box.className = 'widget';
    box.dataset.index = w.index;
    if (w.kind === 'button') {
      const b = document.createElement('button');
      b.textContent = w.forever ? w.display + ' \u221e' : w.display;
      b.disabled = w.disabled;
      if (w.forever) b.className = 'go';
      b.onclick = () => post({op:'button', index:w.index});
      box.appendChild(b);
    } else if (w.kind === 'slider') {
      const lab = document.createElement('label');
      lab.textContent = caption(w);
      const input = document.createElement('input');
      input.type = 'range'; input.min = w.min; input.max = w.max; input.step = w.step || 1; input.value = w.value;
      input.oninput = () => { lab.textContent = w.display + '  ' + input.value + (w.units ? ' ' + w.units : ''); post({op:'slider', index:w.index, value:Number(input.value)}); };
      box.appendChild(lab); box.appendChild(input);
    } else if (w.kind === 'switch') {
      const lab = document.createElement('label');
      const input = document.createElement('input');
      input.type = 'checkbox'; input.checked = w.on;
      input.onchange = () => post({op:'switch', index:w.index, on:input.checked});
      lab.appendChild(input); lab.appendChild(document.createTextNode(' ' + w.display));
      box.appendChild(lab);
    } else if (w.kind === 'monitor') {
      const lab = document.createElement('label');
      lab.textContent = w.display;
      const val = document.createElement('div');
      val.className = 'reading';
      val.textContent = w.reading;
      box.appendChild(lab); box.appendChild(val);
    } else if (w.kind === 'chooser') {
      const lab = document.createElement('label');
      lab.textContent = w.display;
      const sel = document.createElement('select');
      w.choices.forEach((c, i) => {
        const o = document.createElement('option');
        o.value = i; o.textContent = c; if (i === w.current) o.selected = true;
        sel.appendChild(o);
      });
      sel.onchange = () => post({op:'chooser', index:w.index, value:Number(sel.value)});
      box.appendChild(lab); box.appendChild(sel);
    } else if (w.kind === 'input') {
      const lab = document.createElement('label');
      lab.textContent = w.display;
      const input = document.createElement('input');
      input.value = w.text || '';
      input.onchange = () => post({op:'input', index:w.index, text:input.value});
      box.appendChild(lab); box.appendChild(input);
    } else if (w.kind === 'note' || w.kind === 'plot') {
      box.textContent = w.kind === 'plot' ? ('plot not available: ' + w.display) : w.display;
    }
    widgets.appendChild(box);
  }
  document.getElementById('code').value = st.code || '';
  document.getElementById('notes').textContent = st.notes || '';
}
function caption(w) { return w.display + '  ' + w.value + (w.units ? ' ' + w.units : ''); }
function updateReadouts(st) {
  document.querySelectorAll('#widgets .widget').forEach(box => {
    const w = st.widgets.find(x => String(x.index) === box.dataset.index);
    if (!w) return;
    if (w.kind === 'monitor') {
      const val = box.querySelector('.reading');
      if (val) val.textContent = w.reading;
    }
    if (w.kind === 'button') {
      const b = box.querySelector('button');
      if (b) b.disabled = w.disabled;
    }
  });
}
async function poll() {
  try {
    const st = await fetch('/api/state').then(r => r.json());
    document.getElementById('ticks').textContent = st.ticksStarted ? ('ticks: ' + st.ticks) : 'ticks';
    document.getElementById('go').textContent = st.running ? 'stop' : 'go';
    if (st.generation !== gen) { gen = st.generation; build(st); }
    else updateReadouts(st);
    const log = document.getElementById('log');
    const text = (st.log || []).join('\n');
    if (log.textContent !== text) { log.textContent = text; log.scrollTop = log.scrollHeight; }
    const img = document.getElementById('world');
    if (img.dataset.busy !== '1') {
      img.dataset.busy = '1';
      const pre = new Image();
      pre.onload = () => { img.src = pre.src; img.dataset.busy = '0'; };
      pre.onerror = () => { img.dataset.busy = '0'; };
      pre.src = '/frame.png?t=' + Date.now();
    }
  } catch (e) {}
}
document.getElementById('model').onchange = (e) => post({op:'model', name:e.target.value});
document.getElementById('setup').onclick = () => post({op:'setup'});
document.getElementById('go').onclick = () => post({op:'go'});
document.getElementById('speed').oninput = (e) => post({op:'speed', value:Number(e.target.value)});
document.getElementById('apply').onclick = () => post({op:'apply', code:document.getElementById('code').value});
document.getElementById('cmd').onsubmit = (e) => { e.preventDefault(); const line = document.getElementById('line'); post({op:'command', text:line.value}); line.value = ''; };
document.getElementById('tab-iface').onclick = () => { document.getElementById('iface').classList.remove('hidden'); document.getElementById('codepane').classList.add('hidden'); document.getElementById('tab-iface').classList.add('on'); document.getElementById('tab-code').classList.remove('on'); };
document.getElementById('tab-code').onclick = () => { document.getElementById('codepane').classList.remove('hidden'); document.getElementById('iface').classList.add('hidden'); document.getElementById('tab-code').classList.add('on'); document.getElementById('tab-iface').classList.remove('on'); };
setInterval(poll, 150);
poll();
</script>
</body>
</html>`
