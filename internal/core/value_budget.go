package core

import (
	"fmt"
	"reflect"
)

// ConsumeValueBudget checks decoded CQL values before JSON conversion makes
// another copy. It cannot limit frames allocated inside the upstream driver.
// The conservative budget includes JSON string escaping and container overhead.
func ConsumeValueBudget(value any, remaining *int) error {
	if remaining == nil || *remaining < 0 {
		return fmt.Errorf("invalid result budget")
	}
	return valueCost(reflect.ValueOf(value), remaining, 0)
}
func valueCost(v reflect.Value, budget *int, depth int) error {
	if depth > 16 {
		return fmt.Errorf("CQL value nesting exceeds 16")
	}
	if *budget < 64 {
		return fmt.Errorf("CQL decoded result exceeds memory budget")
	}
	*budget -= 64
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return valueCost(v.Elem(), budget, depth+1)
	case reflect.String:
		if v.Len() > *budget/6 {
			return fmt.Errorf("CQL string exceeds result budget")
		}
		*budget -= v.Len() * 6
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.Len() > *budget/2 {
				return fmt.Errorf("CQL binary exceeds result budget")
			}
			*budget -= v.Len() * 2
			return nil
		}
		if v.Len() > *budget/64 {
			return fmt.Errorf("CQL collection exceeds result budget")
		}
		for i := 0; i < v.Len(); i++ {
			if e := valueCost(v.Index(i), budget, depth+1); e != nil {
				return e
			}
		}
	case reflect.Map:
		if v.Len() > *budget/128 {
			return fmt.Errorf("CQL map exceeds result budget")
		}
		it := v.MapRange()
		for it.Next() {
			if e := valueCost(it.Key(), budget, depth+1); e != nil {
				return e
			}
			if e := valueCost(it.Value(), budget, depth+1); e != nil {
				return e
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if e := valueCost(v.Field(i), budget, depth+1); e != nil {
				return e
			}
		}
	}
	return nil
}
