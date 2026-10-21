; Two plots. counts follows the pens' update commands on tick.
; trace is drawn with plotxy and a fixed range.
globals [ wave ]
to setup
  clear-all
  create-turtles 12 [
    setxy random-xcor random-ycor
    set color blue
    set size 1.2
  ]
  set-current-plot "trace"
  clear-plot
  set-plot-x-range 0 80
  set-plot-y-range -5 105
  auto-plot-off
  plot-pen-up
  plotxy 0 50
  plot-pen-down
  reset-ticks
end

to go
  ask turtles [
    right random 50
    left random 50
    forward 0.4
    if random 100 < 2 [ die ]
    if count turtles < 36 and random 100 < 4 [
      hatch 1 [
        right random 360
        set color red
      ]
    ]
  ]
  set wave 50 + 40 * sin (ticks * 12)
  set-current-plot "trace"
  set-current-plot-pen "wave"
  plotxy ticks wave
  tick
end
@#$#@#$#@
GRAPHICS-WINDOW
280
10
592
322
-1
-1
12.0
1
10
1
1
1
0
1
1
1
-8
8
-8
8
1
1
1
ticks
30.0

BUTTON
16
16
110
49
setup
setup
NIL
1
T
OBSERVER
NIL
NIL
NIL
NIL
1

BUTTON
120
16
214
49
go
go
T
1
T
OBSERVER
NIL
NIL
NIL
NIL
0

MONITOR
16
60
120
110
turtles
count turtles
0
1
11

PLOT
16
130
250
300
counts
ticks
turtles
0.0
20.0
0.0
40.0
true
true
"" ""
PENS
"turtles" 1.0 0 -16711936 true "" "plot count turtles"
"near" 1.0 0 -65536 true "" "plot count turtles with [ abs xcor < 3 ]"

PLOT
16
310
250
480
trace
ticks
wave
0.0
80.0
-5.0
105.0
false
false
"" ""
PENS
"wave" 1.0 0 -16776961 true "" ""

@#$#@#$#@
