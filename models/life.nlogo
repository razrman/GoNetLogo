; Conway's Game of Life on the patch grid.
; Birth on 3 live neighbors, survival on 2 or 3. The counts are taken
; before any cell changes, so the generation is simultaneous.
patches-own [ living? live-neighbors ]

to setup
  setup-random
end

to setup-random
  clear-all
  ask patches [
    ifelse random 100 < initial-density [
      birth
    ] [
      die-cell
    ]
  ]
  reset-ticks
end

to setup-blinker
  clear-all
  ask patches [ die-cell ]
  ask patch 0 1 [ birth ]
  ask patch 0 0 [ birth ]
  ask patch 0 -1 [ birth ]
  reset-ticks
end

to birth
  set living? true
  set pcolor white
end

to die-cell
  set living? false
  set pcolor black
end

to go
  ask patches [
    set live-neighbors count neighbors with [ living? ]
  ]
  ask patches [
    ifelse live-neighbors = 3 [
      set living? true
    ] [
      if live-neighbors != 2 [
        set living? false
      ]
    ]
    ifelse living? [
      set pcolor white
    ] [
      set pcolor black
    ]
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
140
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
150
16
274
49
blinker
setup-blinker
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
16
58
140
91
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
110
274
143
initial density
initial-density
0
100
30
1
1
%
HORIZONTAL

MONITOR
16
160
274
210
living cells
count patches with [ living? ]
0
1
11
@#$#@#$#@
Press blinker for a period-2 oscillator, or setup for a random soup.
The world wraps.
@#$#@#$#@
