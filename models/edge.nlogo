; Non-wrapping world. Turtles walk until the edge stops them.
; fd moves one patch at a time and halts when the next step would leave.
globals [ stopped ]
to setup
  clear-all
  create-ordered-turtles 8 [
    setxy 0 0
    pen-down
    set color 15 + who * 10
  ]
  set stopped 0
  reset-ticks
end

to go
  ask turtles [
    if can-move? 1 [ fd 1 ]
  ]
  set stopped count turtles with [ not can-move? 1 ]
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
0
0
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
20
110
53
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
20
214
53
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
70
180
120
stopped
stopped
0
1
11

MONITOR
16
130
180
180
ticks
ticks
0
1
11
@#$#@#$#@
The world does not wrap. Each turtle faces a different direction and
walks with fd 1 while can-move? 1 is true. At the edge it stays on the
last patch that is still inside the world.
@#$#@#$#@
