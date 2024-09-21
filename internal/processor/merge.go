package processor

import (
	"reflect"
)

func mergeReflect(t reflect.Type, a, b, out reflect.Value) {
	switch t.Kind() {
	case reflect.Struct:
		for _, f := range reflect.VisibleFields(t) {
			mergeReflect(f.Type, a.FieldByIndex(f.Index), b.FieldByIndex(f.Index), out.FieldByIndex(f.Index))
		}
	case reflect.Pointer:
		if a.IsNil() {
			out.Set(b)
		} else if b.IsNil() {
			out.Set(a)
		} else {
			out.Set(reflect.New(t.Elem()))
			mergeReflect(t.Elem(), a.Elem(), b.Elem(), out.Elem())
		}
	default:
		if b.IsZero() {
			out.Set(a)
		} else {
			out.Set(b)
		}
	}
}

func Merge[T any](a T, b T) T {
	var out T
	mergeReflect(reflect.TypeFor[T](), reflect.ValueOf(a), reflect.ValueOf(b), reflect.ValueOf(&out).Elem())
	return out
}
