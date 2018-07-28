package mapval

import (
	"reflect"

	"github.com/elastic/beats/libbeat/common"
)

func interfaceToMapStr(o interface{}) common.MapStr {
	newMap := common.MapStr{}
	rv := reflect.ValueOf(o)

	for _, key := range rv.MapKeys() {
		mapV := rv.MapIndex(key)
		keyStr := key.Interface().(string)
		var value interface{} = nil

		if !mapV.IsNil() {
			value = mapV.Interface().(interface{})
		}

		newMap[keyStr] = value
	}
	return newMap
}

func sliceToSliceOfInterfaces(o interface{}) []interface{} {
	rv := reflect.ValueOf(o)
	converted := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		var indexV = rv.Index(i)
		var convertedValue interface{}
		if indexV.Type().Kind() == reflect.Interface {
			if !indexV.IsNil() {
				convertedValue = indexV.Interface().(interface{})
			} else {
				convertedValue = nil
			}
		} else {
			convertedValue = indexV.Interface().(interface{})
		}
		converted[i] = convertedValue
	}
	return converted
}
