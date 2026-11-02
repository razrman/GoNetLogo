# Models Library compatibility

Headless check of every model under `models/library` plus `models/Robby the Robot.nlogo`, using GoNetLogo's own loader and compiler. The library snapshot is the one added in PR #25 (615 `.nlogox` / `.nlogox3d` files). The classic Robby file is a second copy of the library's Robby model, in the old `.nlogo` text format.

Reproduce the run from the repo root:

```bash
go run ./cmd/modelcheck -jobs 3 -timeout 4s -ticks 3 -seed 1
```

`go test ./...` does not scan the library. `MODELCHECK=1 go test ./cmd/modelcheck -run TestLibraryWhenEnabled` compiles every model and skips setup. The numbers below are from the full command: compile, then `setup` when that procedure exists, then up to three `go` calls. Each of those calls gives up after 4 seconds. The random seed is 1. A world longer than 256 patches on either side is compiled and not run (`nl.MaxWorldSide`).

**Compile** means the file loads, the code tab parses, and every button, monitor, and plot or pen setup/update command list parses against that program. An unknown command fails here. An unknown reporter usually does not: the parser treats it as a name, and the failure shows up when `setup` or `go` runs. A model is counted once, under its first error. Fixing that error can expose another, so a group's count is an upper bound on how many models that one change compiles by itself.

No run hit the timeout. The halt check was enough for every model that compiled.

## Summary

| | |
| --- | ---: |
| Total models | 616 |
| File parse OK | 616 |
| Code tab OK | 275 |
| Compile OK | 228 |
| Compile failures | 388 |
| Setup OK | 187 |
| Go OK (3 ticks) | 128 |

All 616 files loaded. There is no `.nlogox` / `.nlogox3d` format failure in this snapshot. The 37 `.nlogox3d` files use `<view3d>`, which the loader already accepts; they fail later, in the language.

Of the 388 compile failures, 47 are only in a button, monitor, or plot command. The code tab of those 47 parsed (`275 - 228`).

Of the 228 that compiled, 213 define `setup` and 187 of those ran it. 22 failed in `setup`. 15 have no `setup` procedure. Five compiled models were not run because the world is larger than 256 patches on one side (four of those have a `setup` procedure). 128 models then completed three `go` ticks. 45 compiled and got through `setup` (or had no `setup`) but failed in `go`. 156 compiled models reported neither a setup error nor a go error; that includes models with no `go` procedure.

Both Robby files compile and finish setup plus three ticks: `models/Robby the Robot.nlogo` and `models/library/Sample Models/Computer Science/Robby the Robot.nlogox`.

## Compile failures, ranked

Rank is models in the group divided by a rough effort weight: **S = 1 hour, M = 4 hours, L = 16 hours**. The letter is the estimate from the task (S under an hour, M a few hours, L a day or more). The fix line is what it would take to get these models past the compiler.

| Rank | Group | Models | Effort | Models / hour | Examples | Fix |
| ---: | --- | ---: | --- | ---: | --- | --- |
| 1 | Observer perspective | 18 | S | 18 | Heatbugs, Traffic Basic, Perspective Example | Add `watch`, `follow`, `watch-me`, and `reset-perspective` with their usual arities. Headless can record the target and otherwise do nothing. |
| 2 | Spelling aliases | 17 | S | 17 | Ethnocentrism (`butfirst`), Party (`if-else`), Ising (`autoplot-off`) | Point `butfirst`/`butlast`, `if-else`, `autoplot-off`/`autoplot-on`, `run-result`, and `penup`/`pendown` at primitives that already exist. |
| 3 | `plot-pen-reset` | 14 | S | 14 | Urban Suite - Positive Feedback, Urban Suite - Structure from Randomness 1, El Farol Extension 1 | Add the command that clears the current plot pen. Most of these failures are in pen update code, not the code tab. |
| 4 | `at-points` | 12 | S | 12 | Percolation, Hex Cells Example, Moore & Von Neumann Example | Add `at-points` for turtles and patches: an agentset from a list of coordinate offsets. |
| 5 | Breed singular as a reporter | 11 | S | 11 | GasLab Free Gas (`ask particle 0`), PageRank (`ask page 0`), Hockey (`a-puck 0`) | Treat `<singular> <number>` like `turtle <number>`, including `ask particle 0`. |
| 6 | HubNet primitives | 34 | M | 8.5 | Disease HubNet, Gridlock HubNet, Polling HubNet | Declare the `hubnet-*` commands and reporters these models call (`hubnet-reset` is the first miss in all 34). A live multiplayer session is a separate project. |
| 7 | HSB / RGB reporters | 8 | S | 8 | HSB and RGB Example, Halo Example, Flocking - Alternative Visualizations | Add `hsb`, `rgb`, `extract-hsb`, `extract-rgb`, `approximate-hsb`, and `approximate-rgb`. |
| 8 | User dialogs | 6 | S | 6 | User Interaction Example, Case Conversion Example, Pac-Man Level Editor | Add `user-input` and `user-yes-or-no?`. Headless can return a default, or fail clearly when the model actually runs. |
| 9 | `link-neighbor?` | 6 | S | 6 | Giant Component, Virus on a Network, Spread of Disease | Add the `link-neighbor?` reporter beside the `link-neighbors` agentset that already exists. |
| 10 | `remove-duplicates` | 6 | S | 6 | Lottery Example, Climate Change, 4 Block Stalagmites | Add `remove-duplicates` for lists. |
| 11 | Parenthesized `ifelse` without else | 6 | S | 6 | Mouse Drag Multiple Example, Chloroplasts and Food, Ant Adaptation | Allow `(ifelse cond1 [c1] cond2 [c2] ...)` to close at `)` when there is no final else block. |
| 12 | Random distributions | 23 | M | 5.8 | K-Means Clustering, Echo, Red Queen | Add `random-normal`, `random-poisson`, `random-exponential`, and `random-gamma`. Today the name is parsed as a variable, so the next token is reported as the error. |
| 13 | Histogram plots | 22 | M | 5.5 | Histogram Example, 4 Blocks, Fairy Circles | Add `histogram` and `set-histogram-num-bars` on plot pens. |
| 14 | One-argument agentset constructors | 5 | S | 5 | Sandpile Simple, ProbLab Genetics, Bug Hunt Disruptions | Let `turtle-set`, `patch-set`, and `link-set` take one agent outside parentheses. They currently always consume a second input, so the next command is parsed as that input. |
| 15 | `subtract-headings` | 4 | S | 4 | Flocking, Flocking (Perspective Demo), Emergent Shapes - Parabola | Add the `subtract-headings` reporter. |
| 16 | `in-cone` | 14 | M | 3.5 | Vision Cone Example, Lightning, Volume Temperature | Add `in-cone` for turtles and patches. The cone test is the work; `in-radius` is the pattern to follow. |
| 17 | `shuffle` | 3 | S | 3 | Voronoi, Rock Paper Scissors, Hotelling's Law | Add the `shuffle` reporter for lists. Agentset shuffling already exists. |
| 18 | `min-n-of` / `max-n-of` | 3 | S | 3 | Simple Economy, Simple Machine Learning, Bacteria Food Hunt | Add `min-n-of` and `max-n-of` next to `min-one-of` and `n-of`. |
| 19 | `tie` | 10 | M | 2.5 | Tie System Example, Label Position Example, Bird Breeder | Implement `tie` and the tie mode it sets, so the tied turtle moves with its partner. |
| 20 | `every` | 9 | M | 2.3 | Bug Hunt Drift, Connected Chemistry 1 Bike Tire, Electrostatics | Add `every`, which runs a command block on a real-time interval. |
| 21 | `sound` extension | 9 | M | 2.3 | Sound Machines, Composer, Beatbox | Accept `sound:name` as one identifier and add the play, drum, and instrument calls these models use. Playback can be silent. |
| 22 | 3D world | 32 | L | 2 | Flocking 3D, Termites 3D, Fireworks 3D | Give the world a z axis: 3-argument `patch` and `patch-at`, `setxyz`, `distancexyz`, `tilt-up` / `tilt-down`, `orbit-down`, and `pzcor`. |
| 23 | File primitives | 7 | M | 1.8 | File Input Example, File Output Example, Network Import Example | Add `file-exists?`, `file-open`, `file-read-line`, `file-close`, and the writers these models call. |
| 24 | `palette` extension | 7 | M | 1.8 | Heat Diffusion - Alternative Visualization, Color Bubbles, Palette Example | Accept `palette:name` and add `scale-gradient` and the other color helpers these models call. |
| 25 | `import-a` and `resource` | 6 | M | 1.5 | Image Resource Example, Bug Hunters Camouflage HubNet, Guppy Spots HubNet | Accept `import-a:name` and `resource:name`, and load bundled images into the drawing or into patch colors. |
| 26 | Import / export of views | 5 | M | 1.3 | Image Import Example, GenEvo 1 Genetic Switch, GenEvo 2 Genetic Drift | Add `import-pcolors`, `import-drawing`, `export-view`, `export-interface`, and `import-world`. |
| 27 | Network layout | 5 | M | 1.3 | Preferential Attachment, Link-Walking Turtles Example, Preferential Attachment Simple | Add `layout-spring`, and `layout-radial` / `layout-tutte` where that is the first missing call. |
| 28 | `carefully` | 5 | M | 1.3 | PNoM 1 Diffusion Sandbox, CA 1D General, Rumor Mill | Add `carefully` with its two command blocks, and the `error` command it is meant to catch. Two further models fail first on the `error` command itself and are in the one-and-two group below. |
| 29 | Other parser leftovers | 5 | M | 1.3 | Doppler, Urban Suite - Cells, Braess Paradox | Five separate parse failures: parenthesized `runresult` (Urban Suite - Cells), `patch-at-heading-and-distance` (Braess Paradox), a bare reporter used as a command (Doppler and the multilayer network), and `ifelse-value`'s argument count (Habitat Preference Behavior). |
| 30 | `sr` extension (R) | 15 | L | 0.9 | Boxplot Example, Linear Regression Example, Diversity Example | Accept `sr:name` and bridge the R calls these models make. This is an external language, not one primitive. The separate `rnd` extension is two models, in the one-and-two group below. |
| 31 | Command tasks | 3 | M | 0.8 | Chaos in a Box, Sandpile, SmoothLife | Accept a command task stored in a variable (`[[x y] -> ...]`) and a procedure parameter that is a task (`to setup [setup-task]`). |
| 32 | `ls` extension (LevelSpace) | 7 | L | 0.4 | Parent Model Example, CRISPR Ecosystem LevelSpace, Model Interactions Example | Accept `ls:name` and run child models. A stub that only parses does not answer `ls:ask`. |
| 33 | System dynamics | 6 | L | 0.4 | Exponential Growth, Logistic Growth, Wolf Sheep Predation (System Dynamics) | The code tab only calls `system-dynamics-setup` / `go`. The diagram in the file is the model; the generated procedures have to be produced or interpreted. |
| 34 | `time` extension | 5 | L | 0.3 | Discrete Event Mousetrap, Discrete Event Whack-a-Mole, Logotimes Example | Accept `time:name` and implement the discrete-event scheduler (`time:create`, `time:go-until`, and the anchors these models use). |
| 35 | `gis` extension | 4 | L | 0.3 | GIS General Examples, GIS Airport Example, GIS Gradient Example | Accept `gis:name` and load shapefiles, projections, and coverage into patches. |
| 36 | `vid` extension | 3 | L | 0.2 | Movie Playing Example, Movie Recording Example, Video Camera Example | Accept `vid:name` and drive movie record / playback. |

The 32 in the 3D row are compile failures whose first error is a 3D primitive or 3D syntax (`setxyz`, `tilt-up`, `orbit-down`, a 3-argument `patch`, and so on). Two further 3D files fail earlier on a shared gap: Airplane Landing Example 3D on `reset-perspective` (row 1), Hydrogen Diffusion 3D on `extract-rgb` (row 7). Three 3D files compile: Life 3D, Rope 3D, and Wave Machine 3D. Rope 3D then fails on `zcor`. Life 3D fails because `go` calls `tick` without `reset-ticks` in `setup`. Wave Machine 3D fails in `setup` on a non-3D expression.

The colon in `sound:play-drum` is not an identifier character yet, so every extension model fails in the lexer before the primitive name is even looked up. Teaching the lexer that `extension:primitive` is one word unlocks nothing by itself; the primitive is then simply missing. The extension rows above include that lexer change.

33 models are in groups of one or two and are not rows above. They are the extensions `csv` (2), `nw` (2), `py` (2), `rnd` (2), `gogo` (2), `view2.5d` (2), and one each of `matrix`, `table`, `bitmap`, `profiler`, and `arduino`; plus `median` (2), `set-plot-pen-color` (2), the `error` command (2), and single primitives (`modes`, `uphill`, `hide-link`, `ask-concurrent`, `__set-line-thickness`, `set-plot-pen-interval`, `__includes`, and a few names that are only the token after a different parse miss). `nw`, `py`, `gogo`, `view2.5d`, and `arduino` are L. `csv`, `table`, `matrix`, `rnd`, and `bitmap` are M. A lone alias or reporter is S. `__includes` is M: load the named `.nls` (there is one, `actions.nls`) and splice it in before parsing. Only one model fails on the directive itself; the other `__includes` model fails first on a colon in its own code.

## Setup and go failures, ranked

These models compiled. The count is how many then failed the first run of `setup` or of `go`. Same effort weights as above.

| Rank | Group | Models | Effort | Models / hour | Examples | Fix |
| ---: | --- | ---: | --- | ---: | --- | --- |
| 1 | `dx` / `dy` | 10 | S | 10 | Connected Chemistry 2 Changing Pressure, Intersecting Lines Example, Connected Chemistry 5 Temperature and Pressure | Add the turtle reporters `dx` and `dy` (and `dz` when a 3D model reaches them). |
| 2 | `mouse-down?` | 5 | S | 5 | Gravitation, Mouse Recording Example, Minesweeper | Add `mouse-down?`, `mouse-xcor`, and `mouse-ycor`. Headless can report the mouse as up and outside the world. |
| 3 | `who` in the closure compiler | 4 | S | 4 | Ants, Ant Lines | `who` is compiled as a zero-argument reporter and never read off the turtle. `evalZero` does not handle it, so `ask turtles [ if who >= 0 ]` fails. Read it from the current agent, the way the tree walker does. |
| 4 | `pi` | 4 | S | 4 | Plotting Example, Rolling Plot Example | Add the `pi` reporter. |
| 5 | `base-colors` | 4 | S | 4 | Fireworks, Patch Clusters Example | Add the `base-colors` reporter (the 14 base hues). |
| 6 | `expected an agent` | 11 | M | 2.8 | Segregation, Wave Machine, Galton Box | Primitives such as `turtles-on` are being handed an agentset (`turtles-on neighbors` in Segregation) and reject it. Accept an agentset anywhere NetLogo does, and check `of` / `myself` on the same path. |
| 7 | World larger than 256 | 5 | M | 1.3 | Mandelbrot (301×301), CA 1D Elementary (257×129), 1D Motion Maps (401×101) | These compile. Setup was not run because `MaxWorldSide` is 256. Raising the cap is a memory and frame-size change, not a language change. |
| 8 | `with [var?]` saw a number | 3 | M | 0.8 | Life, Brian's Brain, CA Stochastic | Setup set `living?` (or the same kind of flag) to a boolean, then `neighbors with [living?]` reported a number. The reporter block is not reading the neighbor's variable. |
| 9 | `hatch` from the observer | 3 | M | 0.8 | Wolf Sheep Predation, Bug Hunt Predators and Invasive Species | `hatch` runs without a turtle as self. The call is inside `ask` in the model, so the ask context is not reaching that procedure. |

Five more compiled models call `tick` in `go` without `reset-ticks` in the `setup` procedure (Life 3D, Turbulence, and three others). That is the checker calling only those two names. The model starts the counter from a different button. It is not a missing primitive.

## Per-model results

`Compile` is the gate above. `Setup` and `Go` are `ok`, `error`, `skipped` (compiled, but not run), or `absent`. On a compile failure, `absent` means the code tab itself did not compile, so procedures were not recorded. `not-run` means the code tab compiled and the procedure exists, but a button, monitor, or plot command failed first. `Go` shows the number of successful ticks when that run finished. The error is the first one.

| Model | Compile | Setup | Go | First error |
| --- | --- | --- | --- | --- |
| `library/3D/Code Examples/Airplane Landing Example 3D.nlogox3d` | fail | not-run | not-run | button "reset-perspective": line 1: nothing named RESET-PERSPECTIVE has been defined |
| `library/3D/Code Examples/Bouncing Balls Example 3D.nlogox3d` | fail | absent | absent | line 59: nothing named SETXYZ has been defined |
| `library/3D/Code Examples/Hill Climbing Example 3D.nlogox3d` | fail | absent | absent | line 27: expected a command |
| `library/3D/Code Examples/Neighborhoods Example 3D.nlogox3d` | fail | absent | absent | line 5: nothing named SETXYZ has been defined |
| `library/3D/Code Examples/Network Example 3D.nlogox3d` | fail | absent | absent | line 32: nothing named TILT-UP has been defined |
| `library/3D/Code Examples/Shapes Example 3D.nlogox3d` | fail | absent | absent | line 13: expected a command block |
| `library/3D/Code Examples/Spherical Path Example 3D.nlogox3d` | fail | absent | absent | line 10: nothing named TILT-UP has been defined |
| `library/3D/Code Examples/Three Loops Example 3D.nlogox3d` | fail | absent | absent | line 14: nothing named TILT-DOWN has been defined |
| `library/3D/Code Examples/Trails Example 3D.nlogox3d` | fail | absent | absent | line 18: nothing named TILT-UP has been defined |
| `library/3D/Code Examples/Turtle Dance Example 3D.nlogox3d` | fail | absent | absent | line 8: nothing named SETXYZ has been defined |
| `library/3D/Code Examples/Turtle Perspective Example 3D.nlogox3d` | fail | absent | absent | line 21: nothing named SETXYZ has been defined |
| `library/3D/Code Examples/Turtle and Observer Motion Example 3D.nlogox3d` | fail | absent | absent | line 28: expected a command block |
| `library/3D/Code Examples/Uniform Distribution on a Sphere Example 3D.nlogox3d` | fail | absent | absent | line 6: nothing named TILT-UP has been defined |
| `library/3D/Sample Models/DLA 3D.nlogox3d` | fail | absent | absent | line 9: expected a command block |
| `library/3D/Sample Models/Fireworks 3D.nlogox3d` | fail | absent | absent | line 20: nothing named ORBIT-DOWN has been defined |
| `library/3D/Sample Models/Flocking 3D Alternate.nlogox3d` | fail | absent | absent | line 19: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Flocking 3D.nlogox3d` | fail | absent | absent | line 14: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Follower 3D.nlogox3d` | fail | absent | absent | line 13: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/GasLab/GasLab Free Gas 3D.nlogox3d` | fail | absent | absent | line 252: nothing named PARTICLE2 has been defined |
| `library/3D/Sample Models/GasLab/GasLab Single Collision 3D.nlogox3d` | fail | absent | absent | line 39: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/GasLab/GasLab Two Gas 3D.nlogox3d` | fail | absent | absent | line 35: nothing named LOAD-SHAPES-3D has been defined |
| `library/3D/Sample Models/Hydrogen Diffusion 3D.nlogox3d` | fail | absent | absent | line 73: nothing named H4-COLOR has been defined |
| `library/3D/Sample Models/Life 3D.nlogox3d` | ok | absent | error | line 28: the tick counter has not been started yet. Use RESET-TICKS |
| `library/3D/Sample Models/Maze 3D.nlogox3d` | fail | absent | absent | line 69: ifelse expected an else block |
| `library/3D/Sample Models/Mousetraps 3D.nlogox3d` | fail | absent | absent | line 22: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Percolation 3D.nlogox3d` | fail | absent | absent | line 10: nothing named ORBIT-DOWN has been defined |
| `library/3D/Sample Models/Preferential Attachment 3D.nlogox3d` | fail | absent | absent | line 39: nothing named TILT-UP has been defined |
| `library/3D/Sample Models/Raindrops 3D.nlogox3d` | fail | absent | absent | line 81: expected a command block |
| `library/3D/Sample Models/Rope 3D.nlogox3d` | ok | ok | error | line 41: line 41: this isn't something you can use SET on: ZCOR |
| `library/3D/Sample Models/Sand 3D.nlogox3d` | fail | absent | absent | line 8: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Sandpile 3D.nlogox3d` | fail | absent | absent | line 10: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Sierpinski Simple 3D.nlogox3d` | fail | absent | absent | line 9: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Sunflower 3D.nlogox3d` | fail | absent | absent | line 16: expected a command |
| `library/3D/Sample Models/Surface Walking 3D.nlogox3d` | fail | absent | absent | line 54: expected ) |
| `library/3D/Sample Models/Termites 3D.nlogox3d` | fail | absent | absent | line 12: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Tree Simple 3D.nlogox3d` | fail | absent | absent | line 13: nothing named SETXYZ has been defined |
| `library/3D/Sample Models/Wave Machine 3D.nlogox3d` | ok | error | skipped | line 22: line 24: nothing named PZCOR has been defined |
| `library/Alternative Visualizations/Ethnocentrism - Alternative Visualization.nlogox` | fail | absent | absent | line 251: nothing named THE-LIST has been defined |
| `library/Alternative Visualizations/Flocking - Alternative Visualizations.nlogox` | fail | absent | absent | line 42: nothing named HEADING has been defined |
| `library/Alternative Visualizations/Heat Diffusion - Alternative Visualization.nlogox` | fail | absent | absent | line 153: this non-standard character is not allowed: ':' |
| `library/Alternative Visualizations/Virus - Alternative Visualization.nlogox` | fail | absent | absent | line 89: nothing named STOP-INSPECTING-DEAD-AGENTS has been defined |
| `library/Alternative Visualizations/Virus - Circle Visualization.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/3D Shapes Example.nlogox` | fail | absent | absent | line 29: nothing named FOLLOW has been defined |
| `library/Code Examples/Ask Ordering Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Ask-Concurrent Example.nlogox` | fail | absent | absent | line 26: nothing named ASK-CONCURRENT has been defined |
| `library/Code Examples/Bounce Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Box Drawing Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Breed Procedures Example.nlogox` | ok | ok | error | line 23: line 1: nothing named AGENTSET has been defined |
| `library/Code Examples/Breeds and Shapes Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Case Conversion Example.nlogox` | fail | absent | absent | line 3: expected a command |
| `library/Code Examples/Circular Path Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Color Chart Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Communication-T-P Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Communication-T-T Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Diffuse Off Edges Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Extensions Examples/csv/CSV Example.nlogox` | fail | absent | absent | line 33: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/gis/GIS Airport Example.nlogox` | fail | absent | absent | line 13: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/gis/GIS General Examples.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/gis/GIS Gradient Example.nlogox` | fail | absent | absent | line 8: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/gogo/GoGoMonitor.nlogox` | fail | absent | absent | button "LED on": line 1: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/gogo/GoGoMonitorSimple.nlogox` | fail | absent | absent | button "a-on": line 1: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/ls/Model Interactions Example.nlogox` | fail | absent | absent | line 10: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/ls/Model Visualizer and Plotter Example.nlogox` | fail | absent | absent | line 6: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/ls/Parent Model Example.nlogox` | fail | absent | absent | line 5: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/matrix/Matrix Example.nlogox` | fail | absent | absent | line 7: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/nw/NW General Examples.nlogox` | fail | absent | absent | line 62: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/palette/Color Bubbles.nlogox` | fail | absent | absent | line 24: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/palette/Color Painting.nlogox` | fail | absent | absent | line 18: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/palette/Color Reveal.nlogox` | fail | absent | absent | line 14: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/palette/Palette Example.nlogox` | fail | absent | absent | line 27: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/profiler/Profiler Example.nlogox` | fail | absent | absent | button "profiler:reset": line 1: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/py/Python Basic Example.nlogox` | fail | absent | absent | line 5: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/py/Python Flocking Clusters.nlogox` | fail | absent | absent | line 12: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/rnd/Rnd Example.nlogox` | fail | absent | absent | line 15: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/Beatbox.nlogox` | fail | absent | absent | line 47: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/Composer.nlogox` | fail | absent | absent | line 60: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/GasLab With Sound.nlogox` | fail | absent | absent | line 42: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/Musical Phrase Example.nlogox` | fail | absent | absent | line 26: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/Percussion Workbench.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sound/Sound Workbench.nlogox` | fail | absent | absent | line 21: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Boxplot Example.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/DataFile Example.nlogox` | fail | absent | absent | line 24: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Diversity Example.nlogox` | fail | absent | absent | line 52: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Example 1.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Interactive Shell.nlogox` | fail | absent | absent | line 23: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Linear Regression Example.nlogox` | fail | absent | absent | line 30: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Package Example.nlogox` | fail | absent | absent | line 21: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Patches to R Matrix.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Plot Example 1.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Plot to File Example.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Plotting.nlogox` | fail | absent | absent | line 7: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/RScript Example.nlogox` | fail | absent | absent | line 23: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Ripleys-K Example.nlogox` | fail | absent | absent | line 36: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Spearman Example.nlogox` | fail | absent | absent | line 24: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/sr/Wilcoxon Example.nlogox` | fail | absent | absent | line 28: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/table/Table Example.nlogox` | fail | absent | absent | line 10: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/time/Discrete Event Mousetrap.nlogox` | fail | absent | absent | line 22: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/time/Discrete Event Whack-a-Mole.nlogox` | fail | absent | absent | line 32: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/time/Logotimes Example.nlogox` | fail | absent | absent | line 34: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/vid/Movie Playing Example.nlogox` | fail | absent | absent | button "vid:movie-select": line 1: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/vid/Movie Recording Example.nlogox` | fail | absent | absent | line 14: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/vid/Video Camera Example.nlogox` | fail | absent | absent | button "copy to drawing layer": line 4: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/view2.5d/2.5d Patch View Example.nlogox` | fail | absent | absent | line 11: this non-standard character is not allowed: ':' |
| `library/Code Examples/Extensions Examples/view2.5d/2.5d Turtle View Example.nlogox` | fail | absent | absent | line 69: this non-standard character is not allowed: ':' |
| `library/Code Examples/File Input Example.nlogox` | fail | absent | absent | line 11: expected ) |
| `library/Code Examples/File Output Example.nlogox` | fail | absent | absent | line 21: expected a command block |
| `library/Code Examples/Fully Connected Network Example.nlogox` | ok | error | skipped | line 8: there is already a link between those turtles |
| `library/Code Examples/Grouping Turtles Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/HSB and RGB Example.nlogox` | fail | absent | absent | line 9: nothing named HUE has been defined |
| `library/Code Examples/Halo Example.nlogox` | fail | absent | absent | line 28: nothing named COLOR has been defined |
| `library/Code Examples/Hatch Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Hex Cells Example.nlogox` | fail | absent | absent | line 24: expected a command block |
| `library/Code Examples/Hex Turtles Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Hill Climbing Example.nlogox` | fail | absent | absent | line 33: nothing named UPHILL has been defined |
| `library/Code Examples/Histogram Example.nlogox` | fail | not-run | not-run | plot "X Coordinates" setup: line 3: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Code Examples/Image Import Example.nlogox` | fail | absent | absent | button "import-pcolors filename": line 1: nothing named IMPORT-PCOLORS has been defined |
| `library/Code Examples/Image Resource Example.nlogox` | fail | absent | absent | button "import-a:pcolors resource:get filename": line 1: this non-standard character is not allowed: ':' |
| `library/Code Examples/Info Tab Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Intersecting Lines Example.nlogox` | ok | error | skipped | line 13: line 30: line 84: line 90: line 90: nothing named DX has been defined |
| `library/Code Examples/Intersecting Links Example.nlogox` | ok | error | skipped | line 20: line 37: line 54: line 54: nothing named LINK-HEADING has been defined |
| `library/Code Examples/Label Position Example.nlogox` | fail | absent | absent | line 21: nothing named TIE has been defined |
| `library/Code Examples/Lattice-Walking Turtles Example.nlogox` | fail | absent | absent | line 21: nothing named AT-POINTS has been defined |
| `library/Code Examples/Line of Sight Example.nlogox` | fail | not-run | not-run | plot "Line of Sight" pen "default" update: line 1: expected a command block |
| `library/Code Examples/Link Breeds Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Link Lattice Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Link-Walking Turtles Example.nlogox` | fail | absent | absent | line 27: nothing named LAYOUT-SPRING has been defined |
| `library/Code Examples/Look Ahead Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Lottery Example.nlogox` | fail | not-run | not-run | plot "mean number of wins per turtle size" update: line 2: nothing named SIZE has been defined |
| `library/Code Examples/Many Regions Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Mobile Aggregation Example.nlogox` | fail | absent | absent | line 42: nothing named HIDE-LINK has been defined |
| `library/Code Examples/Moore & Von Neumann Example.nlogox` | fail | absent | absent | line 18: expected a command block |
| `library/Code Examples/Mouse Drag Multiple Example.nlogox` | fail | absent | absent | line 36: ifelse expected an else block |
| `library/Code Examples/Mouse Drag One Example.nlogox` | fail | absent | absent | line 18: nothing named RESET-PERSPECTIVE has been defined |
| `library/Code Examples/Mouse Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Mouse Recording Example.nlogox` | ok | ok | error | line 13: line 13: nothing named MOUSE-DOWN? has been defined |
| `library/Code Examples/Move Towards Target Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Myself Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Neighborhoods Example.nlogox` | fail | absent | absent | line 58: nothing named AT-POINTS has been defined |
| `library/Code Examples/Network Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Network Import Example.nlogox` | fail | absent | absent | line 21: nothing named FILE-OPEN has been defined |
| `library/Code Examples/Next Patch Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/One Turtle Per Patch Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Ordered Agent Drawing Example.nlogox` | ok | error | skipped | line 30: line 59: line 59: nothing named YCOR has been defined |
| `library/Code Examples/Partners Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Patch Clusters Example.nlogox` | ok | error | skipped | line 15: line 15: nothing named BASE-COLORS has been defined |
| `library/Code Examples/Patch Coordinates Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Patch Outlines Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Perspective Demos/Ants (Perspective Demo).nlogox` | fail | not-run | not-run | button "watch one-of turtles": line 1: nothing named WATCH has been defined |
| `library/Code Examples/Perspective Demos/Flocking (Perspective Demo).nlogox` | fail | absent | absent | line 88: expected ) |
| `library/Code Examples/Perspective Demos/GasLab Gas in a Box (Perspective Demo).nlogox` | fail | absent | absent | line 312: nothing named THE-LIST has been defined |
| `library/Code Examples/Perspective Demos/Termites (Perspective Demo).nlogox` | fail | not-run | not-run | button "watch one-of turtles": line 1: nothing named WATCH has been defined |
| `library/Code Examples/Perspective Example.nlogox` | fail | not-run | not-run | button "follow one-of turtles": line 1: nothing named FOLLOW has been defined |
| `library/Code Examples/Plot Axis Example.nlogox` | fail | not-run | not-run | plot "Position vs. Time" pen "axis" update: line 4: nothing named AUTOPLOT-OFF has been defined |
| `library/Code Examples/Plot Smoothing Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Plotting Example.nlogox` | ok | ok | error | line 19: line 19: nothing named PI has been defined |
| `library/Code Examples/Random Grid Walk Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Random Network Example.nlogox` | ok | absent | absent |  |
| `library/Code Examples/Random Seed Example.nlogox` | fail | absent | absent | line 13: expected a command |
| `library/Code Examples/Random Walk Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Rolling Plot Example.nlogox` | ok | ok | error | line 19: line 19: nothing named PI has been defined |
| `library/Code Examples/Scale-color Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Scatter Example.nlogox` | ok | ok | absent |  |
| `library/Code Examples/Shape Animation Example.nlogox` | fail | not-run | not-run | button "reset-perspective": line 1: nothing named RESET-PERSPECTIVE has been defined |
| `library/Code Examples/State Machine Example.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/Tie System Example.nlogox` | fail | absent | absent | line 16: nothing named TIE has been defined |
| `library/Code Examples/Transparency Example.nlogox` | fail | absent | absent | line 32: nothing named COLOR has been defined |
| `library/Code Examples/Tutorial 3.nlogox` | ok | ok | 3 |  |
| `library/Code Examples/User Interaction Example.nlogox` | fail | absent | absent | line 7: expected a command |
| `library/Code Examples/Vision Cone Example.nlogox` | fail | absent | absent | line 33: expected a command block |
| `library/Code Examples/Wall Following Example.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/BEAGLE Evolution/Bird Breeder.nlogox` | fail | absent | absent | line 497: nothing named TIE has been defined |
| `library/Curricular Models/BEAGLE Evolution/Bug Hunt Coevolution.nlogox` | fail | absent | absent | line 89: nothing named TIE has been defined |
| `library/Curricular Models/BEAGLE Evolution/Bug Hunt Consumers.nlogox` | fail | not-run | not-run | plot "Distribution of Bug Values" update: line 1: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Curricular Models/BEAGLE Evolution/Bug Hunt Drift.nlogox` | fail | absent | absent | line 62: nothing named EVERY has been defined |
| `library/Curricular Models/BEAGLE Evolution/Bug Hunt Predators and Invasive Species.nlogox` | ok | ok | error | line 102: line 293: hatch can only be used by a turtle |
| `library/Curricular Models/BEAGLE Evolution/DNA Protein Synthesis.nlogox` | fail | absent | absent | line 328: expected ) |
| `library/Curricular Models/BEAGLE Evolution/DNA Replication Fork.nlogox` | fail | absent | absent | line 224: nothing named TIE has been defined |
| `library/Curricular Models/BEAGLE Evolution/EACH/Altruism.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/BEAGLE Evolution/EACH/Cooperation.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/BEAGLE Evolution/EACH/Divide The Cake.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/BEAGLE Evolution/Fish Tank Genetic Drift.nlogox` | fail | absent | absent | line 270: nothing named TIE has been defined |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Bird Breeders HubNet.nlogox` | fail | absent | absent | line 99: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Bug Hunters Adaptations HubNet.nlogox` | fail | absent | absent | line 536: this non-standard character is not allowed: ':' |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Bug Hunters Camouflage HubNet.nlogox` | fail | absent | absent | line 160: this non-standard character is not allowed: ':' |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Bug Hunters Competition HubNet.nlogox` | fail | absent | absent | line 50: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Critter Designers HubNet.nlogox` | fail | absent | absent | line 92: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Fish Spotters HubNet.nlogox` | fail | absent | absent | line 72: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/BEAGLE Evolution/HubNet Activities/Guppy Spots HubNet.nlogox` | fail | absent | absent | line 108: this non-standard character is not allowed: ':' |
| `library/Curricular Models/BEAGLE Evolution/Plant Hybridization.nlogox` | fail | absent | absent | line 106: expected a command block |
| `library/Curricular Models/BEAGLE Evolution/Plant Speciation.nlogox` | fail | absent | absent | line 151: expected ) |
| `library/Curricular Models/BEAGLE Evolution/Red Queen.nlogox` | fail | absent | absent | line 19: nothing named INITIAL-POISON-MEAN has been defined |
| `library/Curricular Models/CT-STEM/1D Motion Maps.nlogox` | ok | skipped | skipped | world is 401x101, GoNetLogo allows at most 256 patches on a side |
| `library/Curricular Models/CT-STEM/Calorimetry.nlogox` | fail | absent | absent | line 1044: expected ) |
| `library/Curricular Models/CT-STEM/Free Fall.nlogox` | ok | skipped | absent | world is 41x341, GoNetLogo allows at most 256 patches on a side |
| `library/Curricular Models/CT-STEM/Habitat Preference Behavior.nlogox` | fail | absent | absent | line 92: ifelse-value expected at least 3 inputs |
| `library/Curricular Models/CT-STEM/Hardy Weinberg Equilibrium.nlogox` | ok | ok | error | line 91: line 140: that agent is dead |
| `library/Curricular Models/CT-STEM/Kidney Reabsorption.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/CT-STEM/Mendelian Inheritance.nlogox` | ok | ok | absent |  |
| `library/Curricular Models/CT-STEM/Natural Selection - Camouflage.nlogox` | ok | ok | error | line 90: line 166: that agent is dead |
| `library/Curricular Models/CT-STEM/Pendulum.nlogox` | ok | error | skipped | line 44: line 44: nothing named PI has been defined |
| `library/Curricular Models/CT-STEM/Volume Temperature.nlogox` | fail | absent | absent | line 286: nothing named SQRT has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 1 Bike Tire.nlogox` | fail | absent | absent | line 211: nothing named EVERY has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 2 Changing Pressure.nlogox` | ok | ok | error | line 76: line 163: line 163: nothing named DX has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 3 Circular Particles.nlogox` | fail | absent | absent | line 127: expected a command block |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 4 Number and Pressure.nlogox` | ok | ok | error | line 77: line 165: line 165: nothing named DX has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 5 Temperature and Pressure.nlogox` | ok | ok | error | line 86: line 177: line 177: nothing named DY has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 6 Volume and Pressure.nlogox` | ok | ok | error | line 89: line 172: line 172: nothing named DX has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 7 Ideal Gas Law.nlogox` | ok | ok | error | line 109: line 222: line 222: nothing named DX has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry 8 Gas Particle Sandbox.nlogox` | fail | absent | absent | line 163: nothing named SQRT has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry Atmosphere.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry Gas Combustion.nlogox` | fail | absent | absent | line 259: nothing named PENUP has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry Reversible Reaction.nlogox` | ok | ok | error | line 160: line 355: line 355: nothing named DX has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry Rusting Reaction.nlogox` | fail | absent | absent | line 354: nothing named IN-CONE has been defined |
| `library/Curricular Models/Connected Chemistry/Connected Chemistry Solid Combustion.nlogox` | fail | absent | absent | line 158: nothing named IN-CONE has been defined |
| `library/Curricular Models/GenEvo/GenEvo 1 Genetic Switch.nlogox` | fail | not-run | not-run | button "save screenshot": line 1: nothing named EXPORT-INTERFACE has been defined |
| `library/Curricular Models/GenEvo/GenEvo 2 Genetic Drift.nlogox` | fail | not-run | not-run | button "save screenshot": line 1: nothing named EXPORT-INTERFACE has been defined |
| `library/Curricular Models/GenEvo/GenEvo 3 Genetic Drift and Natural Selection.nlogox` | fail | not-run | not-run | button "save screenshot": line 1: nothing named EXPORT-INTERFACE has been defined |
| `library/Curricular Models/GenEvo/GenEvo 4 Competition.nlogox` | fail | absent | absent | line 33: this non-standard character is not allowed: ':' |
| `library/Curricular Models/GenEvo/Synthetic Biology - Genetic Switch.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Lattice Land/Lattice Land - Explore.nlogox` | fail | absent | absent | line 71: nothing named ERROR has been defined |
| `library/Curricular Models/Lattice Land/Lattice Land - Triangles Dissection.nlogox` | fail | absent | absent | line 67: expected a command |
| `library/Curricular Models/Lattice Land/Lattice Land - Triangles Explore.nlogox` | fail | absent | absent | line 58: expected a command |
| `library/Curricular Models/MaterialSim/MaterialSim Grain Growth.nlogox` | fail | absent | absent | line 77: nothing named AT-POINTS has been defined |
| `library/Curricular Models/Mind the Gap/MTG 1 Equal Opportunities HubNet.nlogox` | fail | absent | absent | line 33: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/Mind the Gap/MTG 2 Random Assignment HubNet.nlogox` | fail | absent | absent | line 33: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/Mind the Gap/MTG 3 Feedback Loop HubNet.nlogox` | fail | absent | absent | line 37: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/ModelSim/Evolution/Bacteria Food Hunt.nlogox` | fail | absent | absent | line 120: nothing named NUMBER-PATCHES-WITH-ALGAE-REGION-1 has been defined |
| `library/Curricular Models/ModelSim/Evolution/Bacteria Hunt Speeds.nlogox` | fail | absent | absent | line 87: nothing named TIE has been defined |
| `library/Curricular Models/ModelSim/Population Biology/Bug Hunt Disruptions.nlogox` | fail | absent | absent | line 214: expected a literal value, got infect-this-bug (use the list primitive to build a list of variables) |
| `library/Curricular Models/ModelSim/Population Biology/Bug Hunt Environmental Changes.nlogox` | fail | absent | absent | line 217: expected a literal value, got infect-this-bug (use the list primitive to build a list of variables) |
| `library/Curricular Models/ModelSim/Population Biology/Bug Hunt Predators and Invasive Species - Two Regions.nlogox` | ok | ok | error | line 213: line 366: hatch can only be used by a turtle |
| `library/Curricular Models/NIELS/Current in a Wire HubNet.nlogox` | fail | absent | absent | line 39: nothing named HUBNET-RESET has been defined |
| `library/Curricular Models/NIELS/Current in a Wire.nlogox` | fail | not-run | not-run | button "Watch An Electron": line 5: nothing named WATCH-ME has been defined |
| `library/Curricular Models/NIELS/Electron Sink.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/NIELS/Electrostatics.nlogox` | fail | absent | absent | line 76: nothing named EVERY has been defined |
| `library/Curricular Models/NIELS/Parallel Circuit.nlogox` | ok | ok | error | line 96: line 129: expected an agent |
| `library/Curricular Models/NIELS/Series Circuit.nlogox` | fail | not-run | not-run | button "Watch An Electron": line 5: nothing named WATCH-ME has been defined |
| `library/Curricular Models/PNoM/PNoM 1 Diffusion Sandbox.nlogox` | fail | absent | absent | line 165: nothing named CAREFULLY has been defined |
| `library/Curricular Models/PNoM/PNoM 2 Diffusion Sensor Sandbox.nlogox` | fail | absent | absent | line 115: nothing named COMMAND-NAME has been defined |
| `library/Curricular Models/PNoM/PNoM 3 Virtual Syringe.nlogox` | fail | absent | absent | line 269: nothing named SQRT has been defined |
| `library/Curricular Models/PNoM/PNoM 4 Virtual Syringe Force Graph.nlogox` | fail | absent | absent | line 267: nothing named SQRT has been defined |
| `library/Curricular Models/PNoM/PNoM 5 Virtual Syringe Temperature Graph.nlogox` | fail | absent | absent | line 271: nothing named SQRT has been defined |
| `library/Curricular Models/PNoM/PNoM 6 Particle Sandbox Gravity.nlogox` | fail | absent | absent | line 184: nothing named SQRT has been defined |
| `library/Curricular Models/PNoM/PNoM 7 Particle Sandbox Intermolecular Forces.nlogox` | fail | absent | absent | line 186: nothing named SQRT has been defined |
| `library/Curricular Models/ProbLab/4 Block Stalagmites.nlogox` | fail | absent | absent | line 266: nothing named SAMPLE-LIST has been defined |
| `library/Curricular Models/ProbLab/4 Block Two Stalagmites.nlogox` | fail | absent | absent | line 584: expected ] |
| `library/Curricular Models/ProbLab/4 Blocks.nlogox` | fail | absent | absent | line 132: nothing named HISTOGRAM has been defined |
| `library/Curricular Models/ProbLab/Histo Blocks.nlogox` | fail | absent | absent | line 95: nothing named EVERY has been defined |
| `library/Curricular Models/ProbLab/Sampler Solo.nlogox` | fail | absent | absent | line 413: expected ) |
| `library/Curricular Models/Urban Suite/Urban Suite - Awareness.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Urban Suite/Urban Suite - Cells.nlogox` | fail | absent | absent | line 53: expected ) |
| `library/Curricular Models/Urban Suite/Urban Suite - Economic Disparity.nlogox` | fail | not-run | not-run | plot "Travel Distance" pen "rich" update: line 1: expected a command |
| `library/Curricular Models/Urban Suite/Urban Suite - Path Dependence.nlogox` | fail | absent | absent | line 153: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Curricular Models/Urban Suite/Urban Suite - Pollution.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Urban Suite/Urban Suite - Positive Feedback.nlogox` | fail | not-run | not-run | plot "Activity distribution" pen "default" update: line 1: nothing named PLOT-PEN-RESET has been defined |
| `library/Curricular Models/Urban Suite/Urban Suite - Recycling.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Urban Suite/Urban Suite - Sprawl Effect.nlogox` | ok | ok | error | line 54: line 91: random expected an integer |
| `library/Curricular Models/Urban Suite/Urban Suite - Structure from Randomness 1.nlogox` | fail | not-run | not-run | plot "Activity distribution" pen "default" update: line 1: nothing named PLOT-PEN-RESET has been defined |
| `library/Curricular Models/Urban Suite/Urban Suite - Structure from Randomness 2.nlogox` | ok | ok | 3 |  |
| `library/Curricular Models/Urban Suite/Urban Suite - Tijuana Bordertowns.nlogox` | fail | absent | absent | line 89: expected ) |
| `library/Curricular Models/epiDEM/epiDEM Basic.nlogox` | fail | absent | absent | line 52: nothing named AVERAGE-RECOVERY-TIME has been defined |
| `library/Curricular Models/epiDEM/epiDEM Travel and Control.nlogox` | fail | absent | absent | line 125: nothing named AVERAGE-ISOLATION-TENDENCY has been defined |
| `library/HubNet Activities/BeeSmart HubNet.nlogox` | fail | absent | absent | line 512: this non-standard character is not allowed: ':' |
| `library/HubNet Activities/Code Examples/Client Overrides Example HubNet.nlogox` | fail | absent | absent | line 9: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Code Examples/Client Perspective Example HubNet.nlogox` | fail | absent | absent | line 11: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Code Examples/Template HubNet.nlogox` | fail | absent | absent | line 20: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Dice Stalagmite HubNet.nlogox` | fail | absent | absent | line 43: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Disease Doctors HubNet.nlogox` | fail | absent | absent | line 47: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Disease HubNet.nlogox` | fail | absent | absent | line 47: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Gridlock HubNet.nlogox` | fail | absent | absent | line 51: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Memory HubNet.nlogox` | fail | absent | absent | line 28: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Minority Game HubNet.nlogox` | fail | absent | absent | line 64: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Polling HubNet.nlogox` | fail | absent | absent | line 32: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Root Beer Game HubNet.nlogox` | fail | absent | absent | line 71: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Sampler HubNet.nlogox` | fail | absent | absent | line 32: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Tragedy of the Commons HubNet.nlogox` | fail | absent | absent | line 62: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Function HubNet.nlogox` | fail | absent | absent | line 61: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Gridlock Alternate HubNet.nlogox` | fail | absent | absent | line 70: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Investments HubNet.nlogox` | fail | absent | absent | line 31: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Oil Cartel Alternate HubNet.nlogox` | fail | absent | absent | line 60: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/PANDA BEAR HubNet.nlogox` | fail | absent | absent | line 29: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Polling Advanced HubNet.nlogox` | fail | absent | absent | line 110: expected a command block |
| `library/HubNet Activities/Unverified/Predator Prey Game HubNet.nlogox` | fail | absent | absent | line 40: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Prisoners Dilemma HubNet.nlogox` | fail | absent | absent | line 56: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Public Good HubNet.nlogox` | fail | absent | absent | line 35: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Restaurants HubNet.nlogox` | fail | absent | absent | line 76: nothing named HUBNET-RESET has been defined |
| `library/HubNet Activities/Unverified/Walking HubNet.nlogox` | fail | absent | absent | line 50: nothing named HUBNET-RESET has been defined |
| `library/IABM Textbook/chapter 0/Turtles Circling Simple.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 1/Ants Simple.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 2/Heroes and Cowards.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 2/Life Simple.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 2/Simple Economy.nlogox` | fail | absent | absent | line 34: nothing named COUNT has been defined |
| `library/IABM Textbook/chapter 3/DLA Extensions/DLA Simple Extension 1.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/DLA Extensions/DLA Simple Extension 2.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/DLA Extensions/DLA Simple Extension 3.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/DLA Extensions/DLA Simple.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/El Farol Extensions/El Farol Extension 1.nlogox` | fail | not-run | not-run | plot "Bar Attendance" pen "threshold" update: line 3: nothing named PLOT-PEN-RESET has been defined |
| `library/IABM Textbook/chapter 3/El Farol Extensions/El Farol Extension 2.nlogox` | fail | not-run | not-run | plot "Bar Attendance" pen "threshold" update: line 3: nothing named PLOT-PEN-RESET has been defined |
| `library/IABM Textbook/chapter 3/El Farol Extensions/El Farol Extension 3.nlogox` | fail | not-run | not-run | plot "Bar Attendance" pen "threshold" update: line 3: nothing named PLOT-PEN-RESET has been defined |
| `library/IABM Textbook/chapter 3/El Farol Extensions/El Farol.nlogox` | fail | not-run | not-run | plot "Bar Attendance" pen "threshold" update: line 3: nothing named PLOT-PEN-RESET has been defined |
| `library/IABM Textbook/chapter 3/Fire Extensions/Fire Simple Extension 1.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/Fire Extensions/Fire Simple Extension 2.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/Fire Extensions/Fire Simple Extension 3.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/Fire Extensions/Fire Simple.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 3/Segregation Extensions/Segregation Simple Extension 1.nlogox` | ok | error | skipped | line 31: line 64: expected an agent |
| `library/IABM Textbook/chapter 3/Segregation Extensions/Segregation Simple Extension 2.nlogox` | ok | error | skipped | line 36: line 69: expected an agent |
| `library/IABM Textbook/chapter 3/Segregation Extensions/Segregation Simple Extension 3.nlogox` | ok | error | skipped | line 34: line 67: expected an agent |
| `library/IABM Textbook/chapter 3/Segregation Extensions/Segregation Simple.nlogox` | ok | error | skipped | line 27: line 60: expected an agent |
| `library/IABM Textbook/chapter 4/Wolf Sheep Simple 1.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 4/Wolf Sheep Simple 2.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 4/Wolf Sheep Simple 3.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 4/Wolf Sheep Simple 4.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 4/Wolf Sheep Simple 5.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 5/Agentset Efficiency.nlogox` | ok | ok | absent |  |
| `library/IABM Textbook/chapter 5/Agentset Ordering.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 5/Communication-T-T Network Example.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 5/Preferential Attachment Simple.nlogox` | fail | absent | absent | line 43: nothing named LAYOUT-SPRING has been defined |
| `library/IABM Textbook/chapter 5/Random Network.nlogox` | ok | ok | absent |  |
| `library/IABM Textbook/chapter 5/Traffic Basic Adaptive Individuals.nlogox` | fail | absent | absent | line 19: nothing named WATCH has been defined |
| `library/IABM Textbook/chapter 5/Traffic Basic Adaptive.nlogox` | fail | absent | absent | line 20: nothing named WATCH has been defined |
| `library/IABM Textbook/chapter 5/Traffic Basic Utility.nlogox` | fail | absent | absent | line 15: nothing named WATCH has been defined |
| `library/IABM Textbook/chapter 5/Traffic Grid Goal.nlogox` | fail | absent | absent | line 363: nothing named WATCH has been defined |
| `library/IABM Textbook/chapter 6/Spread of Disease.nlogox` | fail | absent | absent | line 37: expected ] |
| `library/IABM Textbook/chapter 7/Voting Component Verification.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 7/Voting Sensitivity Analysis.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 8/Arduino Example.nlogox` | fail | absent | absent | line 9: this non-standard character is not allowed: ':' |
| `library/IABM Textbook/chapter 8/Disease With Android Avoidance HubNet.nlogox` | fail | absent | absent | line 47: nothing named HUBNET-RESET has been defined |
| `library/IABM Textbook/chapter 8/Example HubNet.nlogox` | fail | absent | absent | line 6: nothing named HUBNET-RESET has been defined |
| `library/IABM Textbook/chapter 8/Run Example.nlogox` | ok | ok | 3 |  |
| `library/IABM Textbook/chapter 8/Run Result Example.nlogox` | fail | absent | absent | line 21: nothing named STRATEGY has been defined |
| `library/IABM Textbook/chapter 8/Sandpile Simple.nlogox` | fail | absent | absent | line 53: nothing named ACTIVE-PATCHES has been defined |
| `library/IABM Textbook/chapter 8/Simple Machine Learning.nlogox` | fail | absent | absent | line 37: expected a command block |
| `library/IABM Textbook/chapter 8/Simple Viral Marketing.nlogox` | fail | absent | absent | line 25: this non-standard character is not allowed: ':' |
| `library/IABM Textbook/chapter 8/Ticket Sales.nlogox` | fail | absent | absent | line 117: this non-standard character is not allowed: ':' |
| `library/Sample Models/Art/Diffusion Graphics.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Art/Fireworks.nlogox` | ok | ok | error | line 37: line 57: line 57: nothing named BASE-COLORS has been defined |
| `library/Sample Models/Art/Follower.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Art/GenJam - Duple.nlogox` | fail | absent | absent | line 68: this non-standard character is not allowed: ':' |
| `library/Sample Models/Art/Geometron Top-Down.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Art/Kaleidoscope.nlogox` | fail | absent | absent | line 27: nothing named EVERY has been defined |
| `library/Sample Models/Art/Optical Illusions.nlogox` | fail | absent | absent | line 142: nothing named __SET-LINE-THICKNESS has been defined |
| `library/Sample Models/Art/Sound Machines.nlogox` | fail | absent | absent | line 205: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Ant Lines.nlogox` | ok | ok | error | line 68: line 68: nothing named WHO has been defined |
| `library/Sample Models/Biology/Ants.nlogox` | ok | ok | error | line 71: line 71: nothing named WHO has been defined |
| `library/Sample Models/Biology/Autumn.nlogox` | fail | absent | absent | line 50: nothing named START-SUGAR-MEAN has been defined |
| `library/Sample Models/Biology/BeeSmart Hive Finding.nlogox` | fail | absent | absent | line 92: expected a command block |
| `library/Sample Models/Biology/Blood Sugar Regulation.nlogox` | fail | absent | absent | line 89: nothing named HORMONE-MEAN-LIFE has been defined |
| `library/Sample Models/Biology/CRISPR/CRISPR Bacterium LevelSpace.nlogox` | fail | absent | absent | line 105: nothing named WATCH-ME has been defined |
| `library/Sample Models/Biology/CRISPR/CRISPR Bacterium.nlogox` | fail | absent | absent | line 106: nothing named MAP has been defined |
| `library/Sample Models/Biology/CRISPR/CRISPR Ecosystem LevelSpace.nlogox` | fail | absent | absent | line 44: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/CRISPR/CRISPR Ecosystem.nlogox` | fail | absent | absent | line 80: nothing named MIN-REPRODUCE-ENERGY has been defined |
| `library/Sample Models/Biology/Chloroplasts and Food.nlogox` | fail | absent | absent | line 274: ifelse expected an else block |
| `library/Sample Models/Biology/Daisyworld.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Disease Solo.nlogox` | fail | absent | not-run | plot "Number Sick" setup: line 2: nothing named SET-PLOT-PEN-COLOR has been defined |
| `library/Sample Models/Biology/Evolution/Anisogamy.nlogox` | fail | absent | absent | line 184: expected ) |
| `library/Sample Models/Biology/Evolution/Bacterial Infection.nlogox` | fail | absent | absent | line 66: expected a command |
| `library/Sample Models/Biology/Evolution/Baldwin Effect.nlogox` | fail | absent | absent | line 205: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Evolution/Bug Hunt Camouflage.nlogox` | fail | absent | absent | line 222: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Evolution/Bug Hunt Speeds.nlogox` | fail | absent | absent | line 39: nothing named EVERY has been defined |
| `library/Sample Models/Biology/Evolution/Echo.nlogox` | fail | absent | absent | line 125: expected a command |
| `library/Sample Models/Biology/Evolution/Genetic Drift/GenDrift P global.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Evolution/Genetic Drift/GenDrift P local.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Evolution/Genetic Drift/GenDrift T interact.nlogox` | ok | ok | error | line 32: line 46: line 46: nothing named BASE-COLORS has been defined |
| `library/Sample Models/Biology/Evolution/Genetic Drift/GenDrift T reproduce.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Evolution/Mammoths.nlogox` | fail | absent | absent | line 19: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Evolution/Mimicry.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Evolution/Peppered Moths.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Evolution/Sex Ratio Equilibrium.nlogox` | fail | absent | absent | line 47: nothing named INITIAL-AVERAGE-MALE-CHILD-CHANCE has been defined |
| `library/Sample Models/Biology/Evolution/Sunflower Biomorphs.nlogox` | fail | absent | absent | line 101: expected a command |
| `library/Sample Models/Biology/Evolution/Vision Evolution.nlogox` | fail | absent | absent | line 64: nothing named TIE has been defined |
| `library/Sample Models/Biology/Evolution/Wolf Sheep Stride Inheritance.nlogox` | fail | not-run | not-run | plot "sheep stride histogram" setup: line 1: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Biology/Fairy Circles.nlogox` | fail | not-run | not-run | plot "Root Depth Distribution" pen "default" update: line 1: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Biology/Fireflies.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Flocking Vee Formations.nlogox` | fail | absent | absent | line 32: expected ) |
| `library/Sample Models/Biology/Flocking.nlogox` | fail | absent | absent | line 89: expected ) |
| `library/Sample Models/Biology/Food Reward Learning.nlogox` | fail | not-run | not-run | plot "Distribution of Expected Palatability (by Region)" pen "Top" setup: line 1: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Biology/Fur.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/HIV.nlogox` | ok | error | skipped | line 34: line 66: line 98: line 108: random-float expected a nonzero number |
| `library/Sample Models/Biology/Heatbugs.nlogox` | fail | not-run | not-run | button "reset-perspective": line 1: nothing named RESET-PERSPECTIVE has been defined |
| `library/Sample Models/Biology/Honeycomb.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Invasive Plants.nlogox` | fail | absent | absent | line 31: ifelse expected an else block |
| `library/Sample Models/Biology/Membrane Formation.nlogox` | fail | absent | absent | line 59: expected ] |
| `library/Sample Models/Biology/Moths.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Muscle Development.nlogox` | fail | absent | absent | line 144: nothing named CATABOLIC-HORMONE has been defined |
| `library/Sample Models/Biology/Rabbits Grass Weeds.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Repressilator 1D.nlogox` | fail | absent | absent | line 503: nothing named TIE has been defined |
| `library/Sample Models/Biology/Rock Paper Scissors.nlogox` | fail | absent | absent | line 30: nothing named SENTENCE has been defined |
| `library/Sample Models/Biology/Shepherds.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Simple Birth Rates.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Slime Mold Network.nlogox` | fail | absent | absent | line 396: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Slime.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Sunflower Emergent.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Sunflower.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Termites.nlogox` | ok | ok | error | line 18: line 24: line 24: nothing named PCOLOR has been defined |
| `library/Sample Models/Biology/Tumor.nlogox` | ok | ok | error | line 42: line 42: nothing named WHO has been defined |
| `library/Sample Models/Biology/Unverified/Algae.nlogox` | fail | not-run | not-run | plot "algae distribution" pen "default" update: line 1: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Biology/Unverified/Plant Growth.nlogox` | ok | absent | absent |  |
| `library/Sample Models/Biology/Virus Using Discrete Event Simulator.nlogox` | fail | absent | absent | line 39: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Virus.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Biology/Wolf Sheep Predation - Agent Cognition/Wolf Sheep Predation - Micro-Sims Cognitive Model.nlogox` | fail | absent | absent | line 2: expected a declaration or procedure, got __includes |
| `library/Sample Models/Biology/Wolf Sheep Predation - Agent Cognition/Wolf Sheep Predation - Micro-Sims.nlogox` | fail | absent | absent | line 53: this non-standard character is not allowed: ':' |
| `library/Sample Models/Biology/Wolf Sheep Predation.nlogox` | ok | ok | error | line 75: line 108: hatch can only be used by a turtle |
| `library/Sample Models/Chemistry & Physics/Boiling.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chaos in a Box.nlogox` | fail | absent | absent | line 233: expected end of reporter task |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Acids and Bases/Buffer.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Acids and Bases/Strong Acid.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Acids and Bases/Unverified/Diprotic Acid.nlogox` | fail | absent | absent | line 219: nothing named SET-PLOT-PEN-INTERVAL has been defined |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Acids and Bases/Weak Acid.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/B-Z Reaction.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Chemical Equilibrium.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Enzyme Kinetics.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Radical Polymerization.nlogox` | fail | absent | absent | line 79: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Simple Kinetics 1.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Simple Kinetics 2.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Chemical Reactions/Simple Kinetics 3.nlogox` | fail | not-run | not-run | plot "Equilibration Plot" pen "midline" setup: line 2: nothing named AUTOPLOT-OFF has been defined |
| `library/Sample Models/Chemistry & Physics/Diffusion Limited Aggregation/DLA Alternate Linear.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Diffusion Limited Aggregation/DLA Alternate.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Diffusion Limited Aggregation/DLA.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Gas Chromatography.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Adiabatic Piston.nlogox` | ok | ok | error | line 82: line 177: line 177: nothing named DX has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Atmosphere.nlogox` | fail | absent | absent | line 316: nothing named THE-LIST has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Circular Particles.nlogox` | fail | not-run | not-run | plot "Energy Histogram" setup: line 5: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Free Gas.nlogox` | fail | absent | absent | line 39: expected a command block |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Gas in a Box.nlogox` | fail | absent | absent | line 56: expected a command block |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Gravity Box.nlogox` | fail | absent | absent | line 63: expected a command block |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Isothermal Piston.nlogox` | fail | not-run | not-run | plot "Energy Histogram" pen "fast" setup: line 1: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Maxwells Demon.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Single Collision.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/GasLab/GasLab Two Gas.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/GasLab/Unverified/GasLab Heat Box.nlogox` | fail | not-run | not-run | plot "Energy Histogram" pen "fast" setup: line 1: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/Unverified/GasLab Moving Piston.nlogox` | fail | absent | absent | line 269: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/Unverified/GasLab Pressure Box.nlogox` | ok | ok | error | line 69: line 147: line 147: nothing named DX has been defined |
| `library/Sample Models/Chemistry & Physics/GasLab/Unverified/GasLab Second Law.nlogox` | fail | absent | absent | line 91: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Chemistry & Physics/Hydrogen Gas Production.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Ising.nlogox` | fail | not-run | not-run | plot "Magnetization" pen "axis" setup: line 2: nothing named AUTOPLOT-OFF has been defined |
| `library/Sample Models/Chemistry & Physics/Kicked Rotator.nlogox` | ok | error | skipped | line 18: line 80: line 80: nothing named PI has been defined |
| `library/Sample Models/Chemistry & Physics/Kicked Rotators.nlogox` | fail | absent | absent | line 7: this non-standard character is not allowed: ':' |
| `library/Sample Models/Chemistry & Physics/Materials Science/Crystallization/Crystallization Basic.nlogox` | fail | absent | absent | line 35: expected ) |
| `library/Sample Models/Chemistry & Physics/Materials Science/Crystallization/Crystallization Directed.nlogox` | fail | absent | absent | line 44: expected ) |
| `library/Sample Models/Chemistry & Physics/Materials Science/Crystallization/Crystallization Moving.nlogox` | fail | absent | absent | line 36: expected ) |
| `library/Sample Models/Chemistry & Physics/Materials Science/Current in a Semiconductor.nlogox` | fail | absent | absent | line 39: nothing named IF-ELSE has been defined |
| `library/Sample Models/Chemistry & Physics/Materials Science/Dislocation Motion and Deformation.nlogox` | fail | absent | absent | line 94: expected ) |
| `library/Sample Models/Chemistry & Physics/Materials Science/Heat Diffusion.nlogox` | ok | ok | error | line 78: line 107: line 107: nothing named E has been defined |
| `library/Sample Models/Chemistry & Physics/Materials Science/Interstitial Diffusion.nlogox` | fail | absent | absent | line 74: ifelse expected an else block |
| `library/Sample Models/Chemistry & Physics/Materials Science/Molecular Dynamics Lennard-Jones.nlogox` | fail | absent | absent | line 133: ifelse expected an else block |
| `library/Sample Models/Chemistry & Physics/Materials Science/Monte Carlo Lennard-Jones.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Materials Science/Polymer Dynamics.nlogox` | fail | absent | absent | line 42: expected ) |
| `library/Sample Models/Chemistry & Physics/Materials Science/Radical Polymerization with Chains.nlogox` | fail | absent | absent | line 128: nothing named BREED has been defined |
| `library/Sample Models/Chemistry & Physics/Materials Science/Solid Diffusion.nlogox` | fail | absent | absent | line 42: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Chemistry & Physics/Mechanics/Random Balls.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Mechanics/Unverified/Gravitation.nlogox` | ok | ok | error | line 43: line 43: nothing named MOUSE-DOWN? has been defined |
| `library/Sample Models/Chemistry & Physics/Mechanics/Unverified/N-Bodies.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Radioactivity/Decay.nlogox` | fail | absent | absent | line 39: nothing named SET-PLOT-PEN-COLOR has been defined |
| `library/Sample Models/Chemistry & Physics/Radioactivity/Unverified/Reactor Top Down.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Chemistry & Physics/Radioactivity/Unverified/Reactor X-Section.nlogox` | ok | error | skipped | line 29: line 84: repeat expected an integer |
| `library/Sample Models/Chemistry & Physics/Sand.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Sandpile.nlogox` | fail | absent | absent | expected a reporter |
| `library/Sample Models/Chemistry & Physics/Thermostat.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Turbulence.nlogox` | ok | absent | error | line 98: the tick counter has not been started yet. Use RESET-TICKS |
| `library/Sample Models/Chemistry & Physics/Unverified/Osmotic Pressure.nlogox` | fail | absent | absent | line 221: expected a command block |
| `library/Sample Models/Chemistry & Physics/Unverified/Scattering.nlogox` | ok | error | skipped | line 27: line 47: line 63: line 63: nothing named WHO has been defined |
| `library/Sample Models/Chemistry & Physics/Waves/Chladni Figures.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Waves/Lattice Gas Automaton.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Waves/Rope.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Chemistry & Physics/Waves/Unverified/Doppler.nlogox` | fail | absent | absent | line 77: expected a command |
| `library/Sample Models/Chemistry & Physics/Waves/Unverified/Raindrops.nlogox` | ok | error | skipped | line 20: expected an agent |
| `library/Sample Models/Chemistry & Physics/Waves/Unverified/Speakers.nlogox` | ok | error | skipped | line 64: line 131: line 131: nothing named MOUSE-DOWN? has been defined |
| `library/Sample Models/Chemistry & Physics/Waves/Wave Machine.nlogox` | ok | error | skipped | line 37: expected an agent |
| `library/Sample Models/Computer Science/Artificial Neural Net - Multilayer.nlogox` | fail | absent | absent | line 120: expected a command |
| `library/Sample Models/Computer Science/Artificial Neural Net - Perceptron.nlogox` | fail | absent | absent | line 169: expected a command |
| `library/Sample Models/Computer Science/Cellular Automata/Brian's Brain.nlogox` | ok | absent | error | line 48: expected a true/false value, got a number |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Elementary.nlogox` | ok | absent | skipped | world is 257x129, GoNetLogo allows at most 256 patches on a side |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D General.nlogox` | fail | absent | absent | line 515: nothing named CAREFULLY has been defined |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Simple Examples/CA 1D Rule 110.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Simple Examples/CA 1D Rule 250.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Simple Examples/CA 1D Rule 30 Turtle.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Simple Examples/CA 1D Rule 30.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Simple Examples/CA 1D Rule 90.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D Totalistic.nlogox` | ok | skipped | skipped | world is 257x129, GoNetLogo allows at most 256 patches on a side |
| `library/Sample Models/Computer Science/Cellular Automata/CA 1D-Squaring.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Cellular Automata/CA Continuous.nlogox` | ok | absent | error | line 93: the tick counter has not been started yet. Use RESET-TICKS |
| `library/Sample Models/Computer Science/Cellular Automata/CA Stochastic.nlogox` | ok | absent | error | line 111: line 133: expected a true/false value, got a number |
| `library/Sample Models/Computer Science/Cellular Automata/Cyclic CA.nlogox` | fail | absent | absent | line 33: nothing named AT-POINTS has been defined |
| `library/Sample Models/Computer Science/Cellular Automata/Lenia.nlogox` | fail | absent | absent | line 211: this non-standard character is not allowed: ':' |
| `library/Sample Models/Computer Science/Cellular Automata/Life Turtle-Based.nlogox` | ok | absent | error | line 70: the tick counter has not been started yet. Use RESET-TICKS |
| `library/Sample Models/Computer Science/Cellular Automata/Life.nlogox` | ok | absent | error | line 38: expected a true/false value, got a number |
| `library/Sample Models/Computer Science/Dining Philosophers.nlogox` | fail | absent | absent | line 48: nothing named WHO has been defined |
| `library/Sample Models/Computer Science/Hex Cell Aggregation.nlogox` | fail | absent | absent | line 117: nothing named AT-POINTS has been defined |
| `library/Sample Models/Computer Science/K-Means Clustering.nlogox` | fail | absent | absent | line 26: expected a command |
| `library/Sample Models/Computer Science/PageRank.nlogox` | fail | absent | absent | line 49: expected a command block |
| `library/Sample Models/Computer Science/Painted Desert Challenge.nlogox` | ok | ok | error | line 21: line 27: line 27: nothing named CHIP-COLOR has been defined |
| `library/Sample Models/Computer Science/Particle Swarm Optimization.nlogox` | fail | absent | absent | line 55: expected a command |
| `library/Sample Models/Computer Science/Particle Systems/Particle System Basic.nlogox` | ok | error | skipped | line 12: patch expected an integer |
| `library/Sample Models/Computer Science/Particle Systems/Particle System Flame.nlogox` | fail | absent | absent | line 30: nothing named RATE has been defined |
| `library/Sample Models/Computer Science/Particle Systems/Particle System Fountain.nlogox` | fail | absent | absent | line 28: nothing named RATE has been defined |
| `library/Sample Models/Computer Science/Particle Systems/Particle System Waterfall.nlogox` | fail | absent | absent | line 29: nothing named RATE has been defined |
| `library/Sample Models/Computer Science/Robby the Robot.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Robotic Factory.nlogox` | fail | absent | absent | line 45: this non-standard character is not allowed: ':' |
| `library/Sample Models/Computer Science/Simple Genetic Algorithm.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/SmoothLife.nlogox` | fail | absent | absent | line 102: expected end of reporter task |
| `library/Sample Models/Computer Science/Turing Machine 2D.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Unverified/Merge Sort.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Computer Science/Unverified/Simulated Annealing.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Computer Science/Vants.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Computer Science/Wandering Letters.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Earth Science/Climate Change.nlogox` | fail | absent | absent | line 105: expected a command |
| `library/Sample Models/Earth Science/Continental Divide.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Earth Science/Erosion.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Earth Science/Fire.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Earth Science/Grand Canyon.nlogox` | fail | absent | absent | line 30: nothing named FILE-OPEN has been defined |
| `library/Sample Models/Earth Science/Percolation.nlogox` | fail | absent | absent | line 44: expected a command block |
| `library/Sample Models/Earth Science/River Meanders.nlogox` | ok | ok | error | line 39: line 62: expected an agent |
| `library/Sample Models/Earth Science/Unverified/Lightning.nlogox` | fail | absent | absent | line 236: expected a command block |
| `library/Sample Models/Games/Ant Adaptation.nlogox` | fail | absent | absent | line 197: ifelse expected an else block |
| `library/Sample Models/Games/Frogger.nlogox` | fail | absent | absent | line 232: this non-standard character is not allowed: ':' |
| `library/Sample Models/Games/Lunar Lander.nlogox` | fail | absent | absent | line 53: expected ) |
| `library/Sample Models/Games/Minesweeper.nlogox` | ok | ok | error | line 55: line 55: nothing named MOUSE-DOWN? has been defined |
| `library/Sample Models/Games/Pac-Man.nlogox` | fail | absent | absent | line 55: nothing named IMPORT-WORLD has been defined |
| `library/Sample Models/Games/Planarity.nlogox` | fail | absent | absent | line 83: nothing named RESET-PERSPECTIVE has been defined |
| `library/Sample Models/Games/Tetris.nlogox` | fail | absent | absent | line 131: nothing named EVERY has been defined |
| `library/Sample Models/Games/Unverified/Pac-Man Level Editor.nlogox` | fail | absent | absent | line 34: expected a command block |
| `library/Sample Models/Games/Unverified/Projectile Attack.nlogox` | fail | absent | absent | line 34: nothing named TIE has been defined |
| `library/Sample Models/Mathematics/3D Solids.nlogox` | ok | ok | error | line 173: the tick counter has not been started yet. Use RESET-TICKS |
| `library/Sample Models/Mathematics/3D Surface.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Color Fractions.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Conic Sections/Emergent Shapes - Ellipse.nlogox` | fail | absent | absent | line 233: nothing named CAREFULLY has been defined |
| `library/Sample Models/Mathematics/Conic Sections/Emergent Shapes - Hyperbola.nlogox` | fail | absent | absent | line 238: nothing named CAREFULLY has been defined |
| `library/Sample Models/Mathematics/Conic Sections/Emergent Shapes - Parabola.nlogox` | fail | absent | absent | line 59: nothing named HEADING has been defined |
| `library/Sample Models/Mathematics/Division.nlogox` | ok | error | skipped | line 20: line 151: expected an agent |
| `library/Sample Models/Mathematics/Fractals/Koch Curve.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Mathematics/Fractals/L-System Fractals.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Fractals/Mandelbrot.nlogox` | ok | skipped | skipped | world is 301x301, GoNetLogo allows at most 256 patches on a side |
| `library/Sample Models/Mathematics/Fractals/Sierpinski Simple.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Fractals/Star Fractal.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Fractals/Tree Simple.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Mousetraps.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Probability/Binomial Rabbits.nlogox` | fail | absent | absent | line 136: nothing named STEPS has been defined |
| `library/Sample Models/Mathematics/Probability/Birthdays.nlogox` | ok | ok | error | line 45: line 77: line 77: nothing named BIRTHDAY has been defined |
| `library/Sample Models/Mathematics/Probability/Galton Box.nlogox` | ok | ok | error | line 77: expected an agent |
| `library/Sample Models/Mathematics/Probability/ProbLab/Dice Stalagmite.nlogox` | fail | absent | absent | line 148: nothing named SINGLE-OUTCOMES has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Dice.nlogox` | fail | not-run | absent | plot "Successes-per-Sample Distributions" pen "Combination" update: line 2: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Prob Graphs Basic.nlogox` | fail | absent | absent | line 70: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/ProbLab Genetics.nlogox` | fail | absent | absent | line 134: nothing named FISH-FAMILY has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Random Basic.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Probability/ProbLab/Stochastic Patchwork.nlogox` | fail | absent | absent | line 95: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/9-Blocks.nlogox` | fail | absent | absent | line 34: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Central Limit Theorem.nlogox` | fail | absent | absent | line 145: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Equidistant Probability.nlogox` | fail | absent | absent | line 166: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Expected Value Advanced.nlogox` | fail | absent | absent | line 311: nothing named EVERY has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Expected Value.nlogox` | fail | not-run | not-run | plot "Sample Totals" pen "Sample Count" update: line 1: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Partition Permutation Distribution.nlogox` | fail | absent | absent | line 86: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Random Basic Advanced.nlogox` | fail | absent | absent | line 166: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Random Combinations and Permutations.nlogox` | fail | absent | absent | line 465: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Sample Stalagmite.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Probability/ProbLab/Unverified/Shuffle Board.nlogox` | fail | absent | absent | line 240: expected ) |
| `library/Sample Models/Mathematics/Probability/Random Walk 360.nlogox` | ok | ok | error | line 45: line 45: nothing named DX has been defined |
| `library/Sample Models/Mathematics/Probability/Three Doors.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Probability/Unverified/Random Walk Left Right.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Pursuit.nlogox` | ok | error | skipped | setup expected 1 inputs |
| `library/Sample Models/Mathematics/Rugby.nlogox` | ok | ok | error | line 63: line 87: this command can only be used by turtles |
| `library/Sample Models/Mathematics/Turtles Circling.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Mathematics/Unverified/PANDA BEAR Solo.nlogox` | fail | absent | absent | line 95: nothing named EVERY has been defined |
| `library/Sample Models/Mathematics/Unverified/Surface Walking 2D.nlogox` | fail | absent | absent | line 39: expected ] |
| `library/Sample Models/Mathematics/Vector Fields.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Voronoi - Emergent.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Mathematics/Voronoi.nlogox` | fail | absent | absent | line 25: nothing named FILTER has been defined |
| `library/Sample Models/Networks/Diffusion on a Directed Network.nlogox` | fail | absent | absent | line 34: nothing named NEIGHBOR-NODES has been defined |
| `library/Sample Models/Networks/Giant Component.nlogox` | fail | absent | absent | line 109: expected a command block |
| `library/Sample Models/Networks/Preferential Attachment.nlogox` | fail | absent | absent | line 78: nothing named LAYOUT-SPRING has been defined |
| `library/Sample Models/Networks/Small Worlds.nlogox` | fail | absent | absent | line 92: expected ) |
| `library/Sample Models/Networks/Team Assembly.nlogox` | fail | absent | absent | line 169: nothing named LAYOUT-SPRING has been defined |
| `library/Sample Models/Networks/Virus on a Network.nlogox` | fail | absent | absent | line 36: expected ] |
| `library/Sample Models/Philosophy/Signaling Game.nlogox` | fail | absent | absent | line 68: nothing named WORLD-STATE has been defined |
| `library/Sample Models/Psychology/Piaget-Vygotsky Game.nlogox` | fail | absent | absent | line 165: expected ) |
| `library/Sample Models/Social Science/Consumer Choice.nlogox` | fail | absent | absent | line 63: expected ] |
| `library/Sample Models/Social Science/Distribution Center Discrete Event Simulator/Distribution Center Discrete Event Simulator.nlogox` | fail | absent | absent | line 45: this non-standard character is not allowed: ':' |
| `library/Sample Models/Social Science/Economics/Bidding Market.nlogox` | fail | absent | absent | line 151: expected ] |
| `library/Sample Models/Social Science/Economics/Braess Paradox.nlogox` | fail | absent | absent | line 218: expected ] |
| `library/Sample Models/Social Science/Economics/Fruit Wars.nlogox` | fail | absent | absent | line 110: nothing named IF-ELSE has been defined |
| `library/Sample Models/Social Science/Economics/Hotelling's Law.nlogox` | fail | absent | absent | line 136: nothing named LIST has been defined |
| `library/Sample Models/Social Science/Economics/Limited Order Book.nlogox` | fail | absent | absent | line 242: nothing named ERROR has been defined |
| `library/Sample Models/Social Science/Economics/Oil Cartel HubNet.nlogox` | fail | absent | absent | line 57: nothing named HUBNET-RESET has been defined |
| `library/Sample Models/Social Science/Economics/Sugarscape/Sugarscape 1 Immediate Growback.nlogox` | fail | absent | absent | line 42: nothing named FILE-OPEN has been defined |
| `library/Sample Models/Social Science/Economics/Sugarscape/Sugarscape 2 Constant Growback.nlogox` | fail | absent | absent | line 42: nothing named FILE-OPEN has been defined |
| `library/Sample Models/Social Science/Economics/Sugarscape/Sugarscape 3 Wealth Distribution.nlogox` | fail | absent | absent | line 56: nothing named FILE-OPEN has been defined |
| `library/Sample Models/Social Science/Economics/Unverified/Bank Reserves.nlogox` | fail | not-run | not-run | plot "Wealth Distribution Histogram" pen "hist" update: line 8: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Social Science/Economics/Unverified/Cash Flow.nlogox` | fail | not-run | not-run | plot "Wealth Dist Hist" pen "hist" update: line 1: nothing named SET-HISTOGRAM-NUM-BARS has been defined |
| `library/Sample Models/Social Science/Economics/Wealth Distribution.nlogox` | fail | not-run | not-run | plot "Class Histogram" pen "default" update: line 1: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Social Science/Ethnocentrism.nlogox` | fail | absent | absent | line 244: nothing named THE-LIST has been defined |
| `library/Sample Models/Social Science/Hockey.nlogox` | fail | absent | absent | line 56: expected a command |
| `library/Sample Models/Social Science/Language Change.nlogox` | fail | absent | absent | line 197: nothing named LAYOUT-SPRING has been defined |
| `library/Sample Models/Social Science/Minority Game.nlogox` | fail | absent | absent | line 55: expected ] |
| `library/Sample Models/Social Science/Party.nlogox` | fail | absent | absent | line 94: nothing named IF-ELSE has been defined |
| `library/Sample Models/Social Science/Paths.nlogox` | ok | ok | error | line 27: line 35: line 35: nothing named MOUSE-DOWN? has been defined |
| `library/Sample Models/Social Science/Prisoner's Dilemma/Prisoner's Dilemma Basic Evolutionary.nlogox` | fail | absent | absent | line 43: nothing named HISTOGRAM has been defined |
| `library/Sample Models/Social Science/Prisoner's Dilemma/Prisoner's Dilemma Basic.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Social Science/Prisoner's Dilemma/Prisoner's Dilemma N-Person Iterated.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Social Science/Prisoner's Dilemma/Prisoner's Dilemma Two Person Iterated.nlogox` | ok | ok | absent |  |
| `library/Sample Models/Social Science/Rebellion.nlogox` | fail | not-run | not-run | button "watch one": line 3: nothing named WATCH-ME has been defined |
| `library/Sample Models/Social Science/Rumor Mill.nlogox` | fail | absent | absent | line 71: nothing named CAREFULLY has been defined |
| `library/Sample Models/Social Science/Scatter.nlogox` | fail | not-run | not-run | plot "Mobility of Scatterers" update: line 3: nothing named PLOT-PEN-RESET has been defined |
| `library/Sample Models/Social Science/Segregation.nlogox` | ok | error | skipped | line 31: line 63: expected an agent |
| `library/Sample Models/Social Science/Taxi Cabs.nlogox` | fail | absent | absent | line 578: nothing named WAIT-TIME-MEAN has been defined |
| `library/Sample Models/Social Science/Traffic 2 Lanes.nlogox` | fail | absent | absent | line 112: nothing named IN-CONE has been defined |
| `library/Sample Models/Social Science/Traffic Basic.nlogox` | fail | absent | absent | line 18: nothing named WATCH has been defined |
| `library/Sample Models/Social Science/Traffic Grid.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/Social Science/Unverified/Artificial Anasazi.nlogox` | fail | absent | absent | line 108: expected ) |
| `library/Sample Models/Social Science/Unverified/Traffic Intersection.nlogox` | ok | ok | error | line 45: line 64: line 64: nothing named BASE-COLORS has been defined |
| `library/Sample Models/Social Science/Voting.nlogox` | ok | ok | 3 |  |
| `library/Sample Models/System Dynamics/Exponential Growth.nlogox` | fail | absent | absent | line 8: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `library/Sample Models/System Dynamics/Logistic Growth.nlogox` | fail | absent | absent | line 8: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo Hybrid.nlogox` | fail | absent | absent | line 22: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo.nlogox` | fail | absent | absent | line 11: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `library/Sample Models/System Dynamics/Wolf Sheep Predation (Docked Hybrid).nlogox` | fail | absent | absent | line 90: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox` | fail | absent | absent | line 8: nothing named SYSTEM-DYNAMICS-SETUP has been defined |
| `Robby the Robot.nlogo` | ok | ok | 3 |  |
