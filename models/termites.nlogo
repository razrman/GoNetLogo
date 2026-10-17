; Termites gather wood chips into piles.
; A termite that is empty picks up a yellow chip; a termite that is
; carrying drops it on an empty patch. Chip count (patches plus carriers)
; stays constant. Inspired by the Resnick termites model; this source
; is an original write-up of that rule.
turtles-own [ carrying? ]

to setup
  clear-all
  set-default-shape turtles "bug"
  ask patches [
    ifelse random 100 < density [
      set pcolor yellow
    ] [
      set pcolor black
    ]
  ]
  create-turtles population [
    set color white
    set size 1.1
    set carrying? false
    setxy random-xcor random-ycor
  ]
  reset-ticks
end

to go
  ask turtles [ walk ]
  tick
end

to walk
  ifelse carrying? [
    ifelse pcolor = black [
      drop
    ] [
      wiggle
    ]
  ] [
    ifelse pcolor = yellow [
      pick-up
    ] [
      wiggle
    ]
  ]
end

to pick-up
  set pcolor black
  set carrying? true
  set color orange
  wiggle
end

to drop
  set pcolor yellow
  set carrying? false
  set color white
  wiggle
end

to wiggle
  rt random 40
  lt random 40
  fd 1
end

to-report chip-count
  report count patches with [ pcolor = yellow ] + count turtles with [ carrying? ]
end
@#$#@#$#@
GRAPHICS-WINDOW
300
10
612
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
16
130
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
140
16
254
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

SLIDER
16
70
270
103
density
density
0
80
35
1
1
%
HORIZONTAL

SLIDER
16
110
270
143
population
population
1
80
40
1
1
NIL
HORIZONTAL

MONITOR
16
160
270
210
chips
chip-count
0
1
11
@#$#@#$#@
White termites search. Orange termites carry a chip.
Piles form because a carrier keeps walking until it finds a gap,
and an empty termite keeps walking until it finds a chip.
@#$#@#$#@
