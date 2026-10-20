package nl

// goal describes what a bracket or expression is expected to be.
type goal int

const (
	goalAny goal = iota
	goalCmdBlock
	goalRepBlock
	goalList
	goalRepTask
	goalCmdTask
)

const (
	precMin    = -1
	precOr     = 4
	precEq     = 5
	precCmp    = 6
	precAdd    = 7
	precMul    = 8
	precPow    = 9
	precNormal = 10
	precOf     = 11
	precWith   = 13
)

// primSpec is the parser-facing signature of a primitive.
type primSpec struct {
	argc     int
	variadic bool
	optBlock bool
	prec     int
	goals    []goal // length argc; nil means all goalAny
	// ref0 means argument 0 is a variable name, not an expression (diffuse).
	ref0 bool
}

func (s primSpec) goalAt(i int) goal {
	if i >= 0 && i < len(s.goals) {
		return s.goals[i]
	}
	return goalAny
}

// infixSpec is a binary reporter. rightAssoc matters for ^ and of.
type infixSpec struct {
	prec      int
	right     bool
	repBlockR bool // right operand is a reporter block (with)
	repBlockL bool // left operand is a reporter block (of)
}

var infixOps = map[string]infixSpec{
	"or":        {precOr, false, false, false},
	"and":       {precOr, false, false, false},
	"xor":       {precOr, false, false, false},
	"=":         {precEq, false, false, false},
	"!=":        {precEq, false, false, false},
	"<":         {precCmp, false, false, false},
	">":         {precCmp, false, false, false},
	"<=":        {precCmp, false, false, false},
	">=":        {precCmp, false, false, false},
	"+":         {precAdd, false, false, false},
	"-":         {precAdd, false, false, false},
	"*":         {precMul, false, false, false},
	"/":         {precMul, false, false, false},
	"mod":       {precMul, false, false, false},
	"^":         {precPow, true, false, false},
	"of":        {precOf, true, false, true},
	"with":      {precWith, false, true, false},
	"with-min":  {precWith, false, true, false},
	"with-max":  {precWith, false, true, false},
	"in-radius": {precWith, false, false, false},
}

// infixTaskOp is an infix reporter that can be passed as a concise task,
// as in `reduce + xs` or `sort-by > xs`. `of` and `with` stay syntax.
func infixTaskOp(name string) bool {
	switch name {
	case "+", "-", "*", "/", "mod", "^", "=", "!=", "<", ">", "<=", ">=", "and", "or", "xor":
		return true
	default:
		return false
	}
}

// reporters with at least one argument, or variadic. Zero-arg reporters are
// resolved by name at runtime so that agent variables can share their names.
var reporters = map[string]primSpec{
	"turtle":                {argc: 1, prec: precNormal},
	"patch":                 {argc: 2, prec: precNormal},
	"link":                  {argc: 2, prec: precNormal},
	"turtles-on":            {argc: 1, prec: precNormal},
	"turtles-at":            {argc: 2, prec: precNormal},
	"patch-ahead":           {argc: 1, prec: precNormal},
	"patch-at":              {argc: 2, prec: precNormal},
	"patch-left-and-ahead":  {argc: 2, prec: precNormal},
	"patch-right-and-ahead": {argc: 2, prec: precNormal},
	"one-of":                {argc: 1, prec: precNormal},
	"n-of":                  {argc: 2, prec: precNormal},
	"any?":                  {argc: 1, prec: precNormal},
	"all?":                  {argc: 2, prec: precNormal, goals: []goal{goalAny, goalRepBlock}},
	"count":                 {argc: 1, prec: precNormal},
	"sum":                   {argc: 1, prec: precNormal},
	"mean":                  {argc: 1, prec: precNormal},
	"max":                   {argc: 1, prec: precNormal},
	"min":                   {argc: 1, prec: precNormal},
	"variance":              {argc: 1, prec: precNormal},
	"standard-deviation":    {argc: 1, prec: precNormal},
	"map":                   {argc: 2, variadic: true, prec: precNormal, goals: []goal{goalRepTask, goalAny}},
	"filter":                {argc: 2, prec: precNormal, goals: []goal{goalRepTask, goalAny}},
	"reduce":                {argc: 2, prec: precNormal, goals: []goal{goalRepTask, goalAny}},
	"n-values":              {argc: 2, prec: precNormal, goals: []goal{goalAny, goalRepTask}},
	"range":                 {argc: 1, variadic: true, prec: precNormal},
	"random":                {argc: 1, prec: precNormal},
	"random-float":          {argc: 1, prec: precNormal},
	"scale-color":           {argc: 4, prec: precNormal},
	"list":                  {argc: 2, variadic: true, prec: precNormal},
	"sentence":              {argc: 2, variadic: true, prec: precNormal},
	"se":                    {argc: 2, variadic: true, prec: precNormal},
	"fput":                  {argc: 2, prec: precNormal},
	"lput":                  {argc: 2, prec: precNormal},
	"but-first":             {argc: 1, prec: precNormal},
	"bf":                    {argc: 1, prec: precNormal},
	"but-last":              {argc: 1, prec: precNormal},
	"bl":                    {argc: 1, prec: precNormal},
	"first":                 {argc: 1, prec: precNormal},
	"last":                  {argc: 1, prec: precNormal},
	"item":                  {argc: 2, prec: precNormal},
	"length":                {argc: 1, prec: precNormal},
	"empty?":                {argc: 1, prec: precNormal},
	"member?":               {argc: 2, prec: precNormal},
	"position":              {argc: 2, prec: precNormal},
	"reverse":               {argc: 1, prec: precNormal},
	"sort":                  {argc: 1, prec: precNormal},
	"sort-by":               {argc: 2, prec: precNormal, goals: []goal{goalRepTask, goalAny}},
	"sort-on":               {argc: 2, prec: precNormal, goals: []goal{goalRepBlock, goalAny}},
	"remove":                {argc: 2, prec: precNormal},
	"remove-item":           {argc: 2, prec: precNormal},
	"replace-item":          {argc: 3, prec: precNormal},
	"sublist":               {argc: 3, prec: precNormal},
	"word":                  {argc: 2, variadic: true, prec: precNormal},
	"substring":             {argc: 3, prec: precNormal},
	"read-from-string":      {argc: 1, prec: precNormal},
	"not":                   {argc: 1, prec: precNormal},
	"abs":                   {argc: 1, prec: precNormal},
	"round":                 {argc: 1, prec: precNormal},
	"floor":                 {argc: 1, prec: precNormal},
	"ceiling":               {argc: 1, prec: precNormal},
	"precision":             {argc: 2, prec: precNormal},
	"int":                   {argc: 1, prec: precNormal},
	"sqrt":                  {argc: 1, prec: precNormal},
	"sin":                   {argc: 1, prec: precNormal},
	"cos":                   {argc: 1, prec: precNormal},
	"tan":                   {argc: 1, prec: precNormal},
	"asin":                  {argc: 1, prec: precNormal},
	"acos":                  {argc: 1, prec: precNormal},
	"atan":                  {argc: 2, prec: precNormal},
	"ln":                    {argc: 1, prec: precNormal},
	"log":                   {argc: 2, prec: precNormal},
	"exp":                   {argc: 1, prec: precNormal},
	"remainder":             {argc: 2, prec: precNormal},
	"other":                 {argc: 1, prec: precNormal},
	"distance":              {argc: 1, prec: precNormal},
	"distancexy":            {argc: 2, prec: precNormal},
	"towards":               {argc: 1, prec: precNormal},
	"towardsxy":             {argc: 2, prec: precNormal},
	"can-move?":             {argc: 1, prec: precNormal},
	"shade-of?":             {argc: 2, prec: precNormal},
	"ifelse-value":          {argc: 3, variadic: true, prec: 3, goals: []goal{goalAny, goalRepBlock, goalRepBlock}},
	"is-number?":            {argc: 1, prec: precNormal},
	"is-string?":            {argc: 1, prec: precNormal},
	"is-list?":              {argc: 1, prec: precNormal},
	"is-boolean?":           {argc: 1, prec: precNormal},
	"is-agent?":             {argc: 1, prec: precNormal},
	"is-turtle?":            {argc: 1, prec: precNormal},
	"is-patch?":             {argc: 1, prec: precNormal},
	"is-link?":              {argc: 1, prec: precNormal},
	"is-nobody?":            {argc: 1, prec: precNormal},
	"is-agentset?":          {argc: 1, prec: precNormal},
	"is-turtle-set?":        {argc: 1, prec: precNormal},
	"is-patch-set?":         {argc: 1, prec: precNormal},
	"is-link-set?":          {argc: 1, prec: precNormal},
	"max-one-of":            {argc: 2, prec: precNormal, goals: []goal{goalAny, goalRepBlock}},
	"min-one-of":            {argc: 2, prec: precNormal, goals: []goal{goalAny, goalRepBlock}},
	"runresult":             {argc: 1, variadic: true, prec: precNormal, goals: []goal{goalRepTask}},
	"turtle-set":            {argc: 2, variadic: true, prec: precNormal},
	"patch-set":             {argc: 2, variadic: true, prec: precNormal},
	"link-set":              {argc: 2, variadic: true, prec: precNormal},
	"in-link-from":          {argc: 1, prec: precNormal},
	"out-link-to":           {argc: 1, prec: precNormal},
	"link-with":             {argc: 1, prec: precNormal},
	"approximate":           {argc: 2, prec: precNormal},
}

// zeroArgReporters are names that evaluate as reporters when they are not
// variables. Color constants are included.
var zeroArgReporters = map[string]bool{
	"turtles": true, "patches": true, "links": true,
	"turtles-here": true, "patch-here": true,
	"neighbors": true, "neighbors4": true,
	"myself": true, "self": true, "other": false,
	"min-pxcor": true, "max-pxcor": true, "min-pycor": true, "max-pycor": true,
	"world-width": true, "world-height": true, "patch-size": true,
	"ticks": true, "timer": true, "new-seed": true,
	"random-xcor": true, "random-ycor": true, "random-pxcor": true, "random-pycor": true,
	"true": true, "false": true, "nobody": true,
	"netlogo-web?": true,
	"both-ends":    true, "link-neighbors": true, "my-links": true,
	"in-link-neighbors": true, "out-link-neighbors": true,
	"who": true,
}

func init() {
	for name := range colorNames {
		zeroArgReporters[name] = true
	}
}

var commands = map[string]primSpec{
	"clear-all": {argc: 0}, "ca": {argc: 0},
	"clear-turtles": {argc: 0}, "ct": {argc: 0},
	"clear-patches": {argc: 0}, "cp": {argc: 0},
	"clear-globals": {argc: 0},
	"clear-links":   {argc: 0},
	"clear-ticks":   {argc: 0},
	"clear-drawing": {argc: 0}, "cd": {argc: 0},
	"clear-output": {argc: 0},
	"reset-ticks":  {argc: 0},
	"tick":         {argc: 0},
	"tick-advance": {argc: 1},
	"random-seed":  {argc: 1},
	"die":          {argc: 0},
	"fd":           {argc: 1}, "forward": {argc: 1},
	"bk": {argc: 1}, "back": {argc: 1},
	"jump": {argc: 1},
	"rt":   {argc: 1}, "right": {argc: 1},
	"lt": {argc: 1}, "left": {argc: 1},
	"setxy":    {argc: 2},
	"facexy":   {argc: 2},
	"face":     {argc: 1},
	"home":     {argc: 0},
	"pen-down": {argc: 0}, "pd": {argc: 0},
	"pen-up": {argc: 0}, "pu": {argc: 0},
	"hide-turtle": {argc: 0}, "ht": {argc: 0},
	"show-turtle": {argc: 0}, "st": {argc: 0},
	"print":             {argc: 1},
	"show":              {argc: 1},
	"type":              {argc: 1},
	"write":             {argc: 1},
	"output-print":      {argc: 1},
	"output-show":       {argc: 1},
	"user-message":      {argc: 1},
	"resize-world":      {argc: 4},
	"set-default-shape": {argc: 2},
	"set-patch-size":    {argc: 1},
	"diffuse":           {argc: 2, ref0: true},
	"diffuse4":          {argc: 2, ref0: true},
	"beep":              {argc: 0},
	"reset-timer":       {argc: 0},
	"wait":              {argc: 1},
	"display":           {argc: 0},
	"no-display":        {argc: 0},
	"run":               {argc: 1, variadic: true, goals: []goal{goalCmdTask}},
	"create-link-with":  {argc: 1, optBlock: true},
	"create-links-with": {argc: 1, optBlock: true},
	"create-link-to":    {argc: 1, optBlock: true},
	"create-links-to":   {argc: 1, optBlock: true},
	"create-link-from":  {argc: 1, optBlock: true},
	"create-links-from": {argc: 1, optBlock: true},
	"layout-circle":     {argc: 2},
}

// constantReporters can appear inside literal lists.
var constantReporters = map[string]bool{
	"true": true, "false": true, "nobody": true,
}

func init() {
	for name := range colorNames {
		constantReporters[name] = true
	}
}
