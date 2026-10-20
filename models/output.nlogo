; Output area. Original model for GoNetLogo.
; Prints into the OUTPUT widget instead of the command center.
to setup
  clear-all
  output-print "Output area ready."
  output-type "next write is quoted: "
  output-write "widget"
  output-print ""
  reset-ticks
end

to go
  output-print ticks
  tick
  if ticks >= 5 [ stop ]
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
-5
5
-5
5
0
0
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

OUTPUT
16
110
250
230
13

@#$#@#$#@
