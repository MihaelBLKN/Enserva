package network

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

// decodePayload copies a protocol-decoded payload into target. It handles the
// common case where wire structs and gameplay request structs share field names
// or json tag names, avoiding JSON work for hot-path binary messages.
func decodePayload(payload any, target any) error {
	if payload == nil {
		return nil
	}
	if target == nil {
		return errors.New("nil decode target")
	}

	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return errors.New("decode target must be a non-nil pointer")
	}

	sourceValue := reflect.ValueOf(payload)
	if assignReflectValue(targetValue.Elem(), sourceValue) {
		return nil
	}

	// Fallback keeps Decode useful for non-matching application structs while the
	// transport still owns network parsing. The built-in binary hot-path messages
	// are handled above without this conversion.
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func assignReflectValue(target reflect.Value, source reflect.Value) bool {
	if !target.CanSet() || !source.IsValid() {
		return false
	}
	for source.Kind() == reflect.Pointer || source.Kind() == reflect.Interface {
		if source.IsNil() {
			return false
		}
		source = source.Elem()
	}

	if source.Type().AssignableTo(target.Type()) {
		target.Set(source)
		return true
	}
	if source.Type().ConvertibleTo(target.Type()) && safeNumericConversion(target.Kind(), source.Kind()) {
		target.Set(source.Convert(target.Type()))
		return true
	}

	if target.Kind() == reflect.Struct {
		return assignStruct(target, source)
	}
	return false
}

func assignStruct(target reflect.Value, source reflect.Value) bool {
	switch source.Kind() {
	case reflect.Struct:
		fields := sourceFields(source)
		assigned := false
		for index := 0; index < target.NumField(); index++ {
			targetField := target.Type().Field(index)
			if targetField.PkgPath != "" {
				continue
			}
			for _, name := range fieldNames(targetField) {
				sourceField, ok := fields[name]
				if ok && assignReflectValue(target.Field(index), sourceField) {
					assigned = true
					break
				}
			}
		}
		return assigned
	case reflect.Map:
		if source.Type().Key().Kind() != reflect.String {
			return false
		}
		assigned := false
		for index := 0; index < target.NumField(); index++ {
			targetField := target.Type().Field(index)
			if targetField.PkgPath != "" {
				continue
			}
			for _, name := range fieldNames(targetField) {
				sourceField := source.MapIndex(reflect.ValueOf(name))
				if sourceField.IsValid() && assignReflectValue(target.Field(index), sourceField) {
					assigned = true
					break
				}
			}
		}
		return assigned
	default:
		return false
	}
}

func sourceFields(source reflect.Value) map[string]reflect.Value {
	fields := map[string]reflect.Value{}
	sourceType := source.Type()
	for index := 0; index < source.NumField(); index++ {
		field := sourceType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		for _, name := range fieldNames(field) {
			fields[name] = source.Field(index)
		}
	}
	return fields
}

func fieldNames(field reflect.StructField) []string {
	names := []string{field.Name, strings.ToLower(field.Name)}
	if jsonName, ok := jsonFieldName(field); ok {
		names = append(names, jsonName)
	}
	return names
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if tag == "" {
		return "", false
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return "", false
	}
	return name, true
}

func safeNumericConversion(targetKind reflect.Kind, sourceKind reflect.Kind) bool {
	if targetKind == sourceKind {
		return true
	}
	return isNumericKind(targetKind) && isNumericKind(sourceKind)
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
