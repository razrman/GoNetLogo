// Package scalar binds GraphQL's built-in scalars to Go builtins.
//
// gqlgen 0.17.49 looks up Marshal<Scalar> in the package named by
// gqlgen.yml. Those functions are not declared as types in gqlgen's
// own graphql package, so the binding lives here and delegates to it.
package scalar

import "github.com/99designs/gqlgen/graphql"

type (
	Boolean = bool
	Int     = int
	Int32   = int32
	Int64   = int64
	ID      = string
	Float   = float64
	String  = string
)

func MarshalBoolean(v bool) graphql.Marshaler { return graphql.MarshalBoolean(v) }
func UnmarshalBoolean(v any) (bool, error)    { return graphql.UnmarshalBoolean(v) }

func MarshalInt(v int) graphql.Marshaler { return graphql.MarshalInt(v) }
func UnmarshalInt(v any) (int, error)    { return graphql.UnmarshalInt(v) }

func MarshalInt32(v int32) graphql.Marshaler { return graphql.MarshalInt32(v) }
func UnmarshalInt32(v any) (int32, error)    { return graphql.UnmarshalInt32(v) }

func MarshalInt64(v int64) graphql.Marshaler { return graphql.MarshalInt64(v) }
func UnmarshalInt64(v any) (int64, error)    { return graphql.UnmarshalInt64(v) }

func MarshalID(v string) graphql.Marshaler { return graphql.MarshalID(v) }
func UnmarshalID(v any) (string, error)    { return graphql.UnmarshalID(v) }

func MarshalFloat(v float64) graphql.Marshaler { return graphql.MarshalFloat(v) }
func UnmarshalFloat(v any) (float64, error)    { return graphql.UnmarshalFloat(v) }

func MarshalString(v string) graphql.Marshaler { return graphql.MarshalString(v) }
func UnmarshalString(v any) (string, error)    { return graphql.UnmarshalString(v) }
