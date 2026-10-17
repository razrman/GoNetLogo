; Sheep eat grass and wolves eat sheep. A small predator-prey world.
; Original code for the classic energy rules: move, spend a point,
; eat, maybe reproduce, die when energy is gone. Grass regrows on a timer.
breed [ sheep a-sheep ]
breed [ wolves wolf ]
turtles-own [ energy ]
patches-own [ countdown ]

to setup
  clear-all
  set-default-shape sheep "sheep"
  set-default-shape wolves "wolf"
  ask patches [
    ifelse grass? [
      set pcolor one-of [ green brown ]
      set countdown random grass-regrowth-time
    ] [
      set pcolor black
      set countdown 0
    ]
  ]
  create-sheep initial-sheep [
    set color white
    set size 1.3
    set energy random (2 * sheep-gain-from-food)
    setxy random-xcor random-ycor
  ]
  create-wolves initial-wolves [
    set color black
    set size 1.4
    set energy random (2 * wolf-gain-from-food)
    setxy random-xcor random-ycor
  ]
  reset-ticks
end

to go
  if not any? turtles [ stop ]
  ask sheep [ sheep-step ]
  ask wolves [ wolf-step ]
  if grass? [
    ask patches [ regrow ]
  ]
  tick
end

to sheep-step
  move
  set energy energy - 1
  if grass? and pcolor = green [
    set pcolor brown
    set countdown grass-regrowth-time
    set energy energy + sheep-gain-from-food
  ]
  if energy > sheep-reproduce [
    hatch 1 [ set energy (energy / 2) ]
    set energy (energy / 2)
  ]
  if energy < 0 [ die ]
end

to wolf-step
  move
  set energy energy - 1
  if any? sheep-here [
    ask one-of sheep-here [ die ]
    set energy energy + wolf-gain-from-food
  ]
  if energy > wolf-reproduce [
    hatch 1 [ set energy (energy / 2) ]
    set energy (energy / 2)
  ]
  if energy < 0 [ die ]
end

to move
  rt random 50
  lt random 50
  fd 1
end

to regrow
  if pcolor = brown [
    ifelse countdown <= 0 [
      set pcolor green
      set countdown grass-regrowth-time
    ] [
      set countdown countdown - 1
    ]
  ]
end
@#$#@#$#@
GRAPHICS-WINDOW
320
10
632
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
12
120
45
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
130
12
234
45
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

SWITCH
16
55
160
88
grass?
grass?
0
1
-1000

SLIDER
16
96
280
129
initial sheep
initial-sheep
0
80
40
1
1
NIL
HORIZONTAL

SLIDER
16
134
280
167
initial wolves
initial-wolves
0
40
15
1
1
NIL
HORIZONTAL

SLIDER
16
172
280
205
sheep gain
sheep-gain-from-food
0
20
6
1
1
NIL
HORIZONTAL

SLIDER
16
210
280
243
wolf gain
wolf-gain-from-food
0
40
16
1
1
NIL
HORIZONTAL

SLIDER
16
248
280
281
sheep reproduce
sheep-reproduce
1
40
12
1
1
NIL
HORIZONTAL

SLIDER
16
286
280
319
wolf reproduce
wolf-reproduce
1
50
18
1
1
NIL
HORIZONTAL

SLIDER
16
324
280
357
grass regrowth
grass-regrowth-time
0
50
20
1
1
NIL
HORIZONTAL

MONITOR
16
370
140
420
sheep
count sheep
0
1
11

MONITOR
150
370
274
420
wolves
count wolves
0
1
11
@#$#@#$#@
White sheep, black wolves. Green grass turns brown when eaten and
grows back after the regrowth timer. Turn grass off to run without it.
@#$#@#$#@
