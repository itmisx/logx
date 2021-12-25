package logger

import (
	"fmt"
)

type FieldType int

const (
	UnknownType FieldType = iota
	BoolType
	BoolSliceType
	IntType
	IntSliceType
	Int64Type
	Int64SliceType
	Float64Type
	Float64SliceType
	StringType
	StringSliceType
	StringerType
)

type Field struct {
	Key        string
	Type       FieldType
	Bool       bool
	Bools      []bool
	Integer    int
	Integers   []int
	String     string
	Float64    float64
	Integer64  int64
	Integer64s []int64
	Strings    []string
	Float64s   []float64
}

// Bool
func Bool(key string, val bool) Field {
	return Field{Key: key, Type: BoolType, Bool: val}
}

// BoolSlice
func BoolSlice(key string, val []bool) Field {
	return Field{Key: key, Type: BoolSliceType, Bools: val}
}

// Int
func Int(key string, val int) Field {
	return Field{Key: key, Type: IntType, Integer: val}
}

// IntSlice
func IntSlice(key string, val []int) Field {
	return Field{Key: key, Type: IntSliceType, Integers: val}
}

// Int64
func Int64(key string, val int64) Field {
	return Field{Key: key, Type: Int64Type, Integer64: val}
}

// Int64Slice
func Int64Slice(key string, val []int64) Field {
	return Field{Key: key, Type: Int64SliceType, Integer64s: val}
}

// Float64
func Float64(key string, val float64) Field {
	return Field{Key: key, Type: Float64Type, Float64: val}
}

// Float64Slice
func Float64Slice(key string, val []float64) Field {
	return Field{Key: key, Type: Float64SliceType, Float64s: val}
}

// String
func String(key string, val string) Field {
	return Field{Key: key, Type: StringType, String: val}
}

// StringSlice
func StringSlice(key string, val []string) Field {
	return Field{Key: key, Type: StringSliceType, Strings: val}
}

// Stringer
func Stringer(key string, val fmt.Stringer) Field {
	return Field{Key: key, Type: StringerType, String: val.String()}
}
