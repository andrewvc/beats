package mapval

import (
	"reflect"

	"github.com/elastic/beats/libbeat/common"
)

func interfaceToMapStr(o interface{}) common.MapStr {
	newMap := common.MapStr{}
	rv := reflect.ValueOf(o)

	for _, key := range rv.MapKeys() {
		value := rv.MapIndex(key).Interface().(interface{})
		newMap[key.Interface().(string)] = value
	}
	return newMap
}

func sliceToSliceOfInterfaces(o interface{}) []interface{} {
	rv := reflect.ValueOf(o)
	converted := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		c := rv.Index(i).Interface().(interface{})
		converted[i] = c
	}
	return converted
}
