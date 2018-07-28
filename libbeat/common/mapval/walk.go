// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package mapval

import (
	"reflect"

	"github.com/elastic/beats/libbeat/common"
)

type walkObserverInfo struct {
	key     PathComponent
	value   interface{}
	rootMap common.MapStr
	path    Path
}

// walkObserver functions run once per object in the tree.
type walkObserver func(info walkObserverInfo)

// walk is a shorthand way to walk a tree.
func walk(m common.MapStr, wo walkObserver) {
	walkFullMap(m, m, Path{}, wo)
}

func walkFull(o interface{}, root common.MapStr, path Path, wo walkObserver) {
	rt := reflect.TypeOf(o)
	switch rt.Kind() {
	case reflect.Map:
		newMap := common.MapStr{}
		rv := reflect.ValueOf(o)

		for _, key := range rv.MapKeys() {
			value := rv.MapIndex(key).Interface().(interface{})
			newMap[key.Interface().(string)] = value
		}
		walkFullMap(newMap, root, path, wo)
	case reflect.Slice:
		rv := reflect.ValueOf(o)
		converted := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			c := rv.Index(i).Interface().(interface{})
			converted[i] = c
		}
		for idx, v := range converted {
			// Add array subscript to last path element
			newPath := path.ExtendSlice(idx)
			// Not a real key, but close to it

			wo(walkObserverInfo{newPath.Last(), v, root, newPath})
			walkFull(v, root, newPath, wo)
		}
	default:
		wo(walkObserverInfo{path.Last(), o, root, path})
	}

	/*
		switch oTyped := o.(type) {
		case common.MapStr:
			walkFullMap(oTyped, root, path, wo)
		case Map:
			walkFullMap(common.MapStr(oTyped), root, path, wo)
		case []interface{}:
			for idx, v := range oTyped {
				// Add array subscript to last path element
				pLen := len(path)
				newPath := make([]string, pLen)
				copy(newPath, path)
				newPath[pLen] = newPath[pLen] + fmt.Sprintf("[%d]", idx)

				walkFull(v, root, newPath, wo)
			}
			return
		default:
			fmt.Printf("ENCOUNTERED A %v\n", oTyped)
		}
	*/
}

// walkFullMap walks the given MapStr tree.
func walkFullMap(m common.MapStr, root common.MapStr, path Path, wo walkObserver) {
	for k, v := range m {
		//TODO: Handle this error
		additionalPath, _ := ParsePath(k)
		newPath := path.Concat(additionalPath)

		walkFull(v, root, newPath, wo)
	}
}
