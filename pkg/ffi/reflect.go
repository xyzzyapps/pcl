package ffi

import (
	"fmt"
	"reflect"
	"pcl/pkg/core"
)

// CallGoFunc invokes any arbitrary Go function using reflection and marshals PCL Values to Go types.
func CallGoFunc(fn interface{}, args []*core.Value) (*core.Value, error) {
	val := reflect.ValueOf(fn)
	if val.Kind() != reflect.Func {
		return nil, fmt.Errorf("target is not a Go function (kind: %s)", val.Kind())
	}

	typ := val.Type()
	numIn := typ.NumIn()
	isVariadic := typ.IsVariadic()

	// Validate argument count
	if isVariadic {
		if len(args) < numIn-1 {
			return nil, fmt.Errorf("function expects at least %d arguments, got %d", numIn-1, len(args))
		}
	} else {
		if len(args) != numIn {
			return nil, fmt.Errorf("function expects %d arguments, got %d", numIn, len(args))
		}
	}

	inValues := make([]reflect.Value, 0, len(args))

	for i, arg := range args {
		var targetType reflect.Type
		if isVariadic && i >= numIn-1 {
			targetType = typ.In(numIn - 1).Elem()
		} else {
			targetType = typ.In(i)
		}

		goVal, err := convertValueToGo(arg, targetType)
		if err != nil {
			return nil, fmt.Errorf("arg %d conversion error: %w", i, err)
		}
		inValues = append(inValues, goVal)
	}

	outValues := val.Call(inValues)

	// Handle return values
	if len(outValues) == 0 {
		return core.NewNull(), nil
	}

	// Check if last return is an error
	lastOut := outValues[len(outValues)-1]
	if lastOut.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		if !lastOut.IsNil() {
			errVal := lastOut.Interface().(error)
			return nil, errVal
		}
		// If error is nil, discard it from return values
		outValues = outValues[:len(outValues)-1]
	}

	if len(outValues) == 0 {
		return core.NewNull(), nil
	}
	if len(outValues) == 1 {
		return convertGoToValue(outValues[0].Interface()), nil
	}

	// Multiple return values -> return as PCL List
	items := make([]*core.Value, len(outValues))
	for i, out := range outValues {
		items[i] = convertGoToValue(out.Interface())
	}
	return core.NewList(items...), nil
}

func convertValueToGo(val *core.Value, targetType reflect.Type) (reflect.Value, error) {
	if val == nil || val.Type() == core.TypeNull {
		return reflect.Zero(targetType), nil
	}

	if targetType == reflect.TypeOf((*core.Value)(nil)) {
		return reflect.ValueOf(val), nil
	}

	// If target is interface{}, return native Go interface
	if targetType.Kind() == reflect.Interface && targetType.NumMethod() == 0 {
		return reflect.ValueOf(val.ToNative()), nil
	}

	switch targetType.Kind() {
	case reflect.String:
		return reflect.ValueOf(val.String()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := val.AsInt()
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(targetType).Elem()
		out.SetInt(i)
		return out, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := val.AsInt()
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(targetType).Elem()
		out.SetUint(uint64(i))
		return out, nil

	case reflect.Float32, reflect.Float64:
		f, err := val.AsFloat()
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(targetType).Elem()
		out.SetFloat(f)
		return out, nil

	case reflect.Bool:
		return reflect.ValueOf(val.AsBool()), nil

	case reflect.Slice:
		if targetType.Elem().Kind() == reflect.Uint8 {
			// []byte
			return reflect.ValueOf([]byte(val.String())), nil
		}
		// Convert list to slice
		if val.Type() != core.TypeList {
			return reflect.Value{}, fmt.Errorf("expected list, got %s", val.Type())
		}
		slice := reflect.MakeSlice(targetType, len(val.ListVal), len(val.ListVal))
		elemType := targetType.Elem()
		for i, el := range val.ListVal {
			converted, err := convertValueToGo(el, elemType)
			if err != nil {
				return reflect.Value{}, err
			}
			slice.Index(i).Set(converted)
		}
		return slice, nil

	case reflect.Map:
		if val.Type() != core.TypeDict {
			return reflect.Value{}, fmt.Errorf("expected dict, got %s", val.Type())
		}
		m := reflect.MakeMap(targetType)
		keyType := targetType.Key()
		elemType := targetType.Elem()
		for k, v := range val.DictVal {
			kVal, err := convertValueToGo(core.NewString(k), keyType)
			if err != nil {
				return reflect.Value{}, err
			}
			vVal, err := convertValueToGo(v, elemType)
			if err != nil {
				return reflect.Value{}, err
			}
			m.SetMapIndex(kVal, vVal)
		}
		return m, nil

	default:
		return reflect.ValueOf(val.ToNative()), nil
	}
}

func convertGoToValue(val interface{}) *core.Value {
	if val == nil {
		return core.NewNull()
	}
	return core.FromNative(val)
}
