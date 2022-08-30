package logger

import (
	"errors"
	"fmt"
)

type FieldType int

const (
	nnknownType FieldType = iota
	boolType
	boolSliceType
	intType
	intSliceType
	int64Type
	int64SliceType
	float64Type
	float64SliceType
	stringType
	stringSliceType
	stringerType
	anyType
	errType
)

type Field struct {
	Key        	string
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
	Any interface{}
}

// Bool
func Bool(key string, val bool) Field {
	return Field{Key: key, Type: boolType, Bool: val}
}

// BoolSlice
func BoolSlice(key string, val []bool) Field {
	return Field{Key: key, Type: boolSliceType, Bools: val}
}

// Int
func Int(key string, val int) Field {
	return Field{Key: key, Type: intType, Integer: val}
}

// IntSlice
func IntSlice(key string, val []int) Field {
	return Field{Key: key, Type: intSliceType, Integers: val}
}

// Int64
func Int64(key string, val int64) Field {
	return Field{Key: key, Type: int64Type, Integer64: val}
}

// Int64Slice
func Int64Slice(key string, val []int64) Field {
	return Field{Key: key, Type: int64SliceType, Integer64s: val}
}

// Float64
func Float64(key string, val float64) Field {
	return Field{Key: key, Type: float64Type, Float64: val}
}

// Float64Slice
func Float64Slice(key string, val []float64) Field {
	return Field{Key: key, Type: float64SliceType, Float64s: val}
}

// String
func String(key string, val string) Field {
	return Field{Key: key, Type: stringType, String: val}
}

// StringSlice
func StringSlice(key string, val []string) Field {
	return Field{Key: key, Type: stringSliceType, Strings: val}
}

// Stringer
func Stringer(key string, val fmt.Stringer) Field {
	return Field{Key: key, Type: stringerType, String: val.String()}
}

// Any
func Any(key string, val interface{}) Field {
	return Field{Key: key, Type: anyType, Any: val}
}

// Err
func Err(err error) Field {
	if err == nil {
		err = errors.New("nil")
	}
	return Field{Key: "error", Type: stringType, String: err.Error()}
}
