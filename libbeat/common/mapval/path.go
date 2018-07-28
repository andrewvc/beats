package mapval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"reflect"

	"github.com/elastic/beats/libbeat/common"
)

type PathComponentType int

const (
	PCMap = iota
	PCSlice
)

type PathComponent struct {
	Type  int    // One of PCMap or PCSlice
	Key   string // Populated for maps
	Index int    // Populated for slices
}

func (pc PathComponent) String() string {
	if pc.Type == PCSlice {
		return fmt.Sprintf("[%d]", pc.Index)
	}
	return pc.Key
}

type Path []PathComponent

func (p Path) ExtendSlice(index int) Path {
	return p.extend(
		PathComponent{PCSlice, "", index},
	)
}

func (p Path) ExtendMap(key string) Path {
	return p.extend(
		PathComponent{PCMap, key, -1},
	)
}

func (p Path) extend(pc PathComponent) Path {
	out := make(Path, len(p)+1)
	copy(out, p)
	out[len(p)] = pc
	return out
}

func (p Path) Concat(other Path) Path {
	out := make(Path, 0, len(p)+len(other))
	out = append(out, p...)
	return append(out, other...)
}

func (p Path) String() string {
	out := make([]string, len(p))
	for idx, pc := range p {
		out[idx] = pc.String()
	}
	return strings.Join(out, ".")
}

func (p Path) Last() PathComponent {
	return p[len(p)-1]
}

func (p Path) GetFrom(m common.MapStr) (exists bool, value interface{}) {
	value = m
	for _, pc := range p {
		switch reflect.TypeOf(value).Kind() {
		case reflect.Map:
			converted := interfaceToMapStr(value)
			value, exists = converted[pc.Key]
		case reflect.Slice:
			converted := sliceToSliceOfInterfaces(value)
			if pc.Index < len(converted) {
				exists = true
				value = converted[pc.Index]
			} else {
				exists = false
				value = nil
			}
		default:
			panic("Unexpected type")
		}
	}

	return exists, value
}

var arrMatcher = regexp.MustCompile("\\[(\\d+)\\]")

type InvalidPathString string

func (ps InvalidPathString) Error() string {
	return fmt.Sprintf("Invalid path Path: %#v", ps)
}

func ParsePath(in string) (p Path, err error) {
	keyParts := strings.Split(in, ".")

	p = make(Path, len(keyParts))
	for idx, part := range keyParts {
		r := arrMatcher.FindStringSubmatch(part)
		pc := PathComponent{}
		if len(r) > 0 {
			pc.Type = PCSlice
			// Cannot fail, validated by regexp already
			pc.Index, err = strconv.Atoi(r[1])
			if err != nil {
				return p, err
			}
		} else if len(part) > 0 {
			pc.Type = PCMap
			pc.Key = part
		} else {
			return p, InvalidPathString(in)
		}

		p[idx] = pc
	}

	return p, nil
}

func MustParsePath(in string) Path {
	out, err := ParsePath(in)
	if err != nil {
		panic(err)
	}
	return out
}
