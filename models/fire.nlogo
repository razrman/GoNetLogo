; Forest fire. Original model for GoNetLogo.
; The spread rule is the well-known percolation fire: a burning tree
; ignites green orthogonal neighbors, then cools. Inspired by Wilensky's
; Fire model; this source is not that file.
globals [ density burned-trees ]
breed [ fires fire ]
breed [ embers ember ]

to setup
  clear-all
  set-default-shape turtles "square"
  set burned-trees 0
  ask patches [
    if random 100 < density [
      set pcolor green
    ]
  ]
  ask one-of patches [
    ignite
  ]
  reset-ticks
end

to ignite
  if pcolor = green [
    set burned-trees burned-trees + 1
  ]
  set pcolor black
  sprout-fires 1 [
    set color red
    set size 0.8
  ]
end

to go
  if not any? turtles [ stop ]
  ask fires [
    ask neighbors4 with [ pcolor = green ] [
      ignite
    ]
    set breed embers
  ]
  ask embers [
    set color color - 0.4
    if color < red - 2 [ die ]
  ]
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
-12
12
-12
12
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

SLIDER
16
70
250
103
density
density
0
100
60
1
1
%
HORIZONTAL

MONITOR
16
120
250
170
burned trees
burned-trees
0
1
11

MONITOR
16
180
250
230
fires
count fires
0
1
11
@#$#@#$#@
A tree catches when a fire is on an orthogonal neighbor.
The density slider is the chance a patch starts as a tree.
Embers cool and disappear; the black patches are what burned.
@#$#@#$#@
