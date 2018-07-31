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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"fmt"

	"github.com/elastic/beats/libbeat/common"
)

func assertValidator(t *testing.T, validator Validator, input common.MapStr) {
	res, err := validator(input)
	require.NoError(t, err)

	assertResults(t, res)
}

// assertResults validates the schema passed successfully.
func assertResults(t *testing.T, r *Results) *Results {
	for _, err := range r.Errors() {
		assert.NoError(t, err)
	}
	return r
}

func TestFlat(t *testing.T) {
	m := common.MapStr{
		"foo": "bar",
		"baz": 1,
	}

	results, err := Schema(Map{
		"foo": "bar",
		"baz": IsIntGt(0),
	})(m)
	require.NoError(t, err)

	assertResults(t, results)
}

func TestBadFlat(t *testing.T) {
	m := common.MapStr{}

	fakeT := new(testing.T)

	results, err := Schema(Map{
		"notafield": IsDuration,
	})(m)
	require.NoError(t, err)

	assertResults(fakeT, results)

	assert.True(t, fakeT.Failed())

	result := results.Fields["notafield"][0]
	assert.False(t, result.Valid)
	assert.Equal(t, result, KeyMissingVR)
}

func TestNested(t *testing.T) {
	m := common.MapStr{
		"foo": common.MapStr{
			"bar": "baz",
			"dur": time.Duration(100),
		},
	}

	results, err := Schema(Map{
		"foo": Map{
			"bar": "baz",
		},
		"foo.dur": IsDuration,
	})(m)

	require.NoError(t, err)

	assertResults(t, results)

	assert.Len(t, results.Fields, 2, "One result per matcher")
}

func TestComposition(t *testing.T) {
	m := common.MapStr{
		"foo": "bar",
		"baz": "bot",
	}

	fooValidator := Schema(Map{"foo": "bar"})
	bazValidator := Schema(Map{"baz": "bot"})
	composed := Compose(fooValidator, bazValidator)

	// Test that the validators work individually
	assertValidator(t, fooValidator, m)
	assertValidator(t, bazValidator, m)

	// Test that the composition of them works
	assertValidator(t, composed, m)

	composedRes, _ := composed(m)
	assert.Len(t, composedRes.Fields, 2)

	badValidator := Schema(Map{"notakey": "blah"})
	badComposed := Compose(badValidator, composed)

	fakeT := new(testing.T)
	assertValidator(fakeT, badComposed, m)
	badComposedRes, _ := badComposed(m)

	assert.Len(t, badComposedRes.Fields, 3)
	assert.True(t, fakeT.Failed())
}

func TestStrictFunc(t *testing.T) {
	m := common.MapStr{
		"foo": "bar",
		"baz": "bot",
		"nest": common.MapStr{
			"very": common.MapStr{
				"deep": "true",
			},
		},
	}

	validValidator := Schema(Map{
		"foo": "bar",
		"baz": "bot",
		"nest": Map{
			"very": Map{
				"deep": "true",
			},
		},
	})

	assertValidator(t, validValidator, m)

	partialValidator := Schema(Map{
		"foo": "bar",
	})

	// Should pass, since this is not a strict check
	assertValidator(t, partialValidator, m)

	res, err := Strict(partialValidator)(m)
	require.NoError(t, err)

	assert.Equal(t, []ValueResult{StrictFailureVR}, res.DetailedErrors().Fields["baz"])
	assert.Equal(t, []ValueResult{StrictFailureVR}, res.DetailedErrors().Fields["nest.very.deep"])
	assert.Nil(t, res.DetailedErrors().Fields["bar"])
	assert.False(t, res.Valid)
}

func TestOptional(t *testing.T) {
	m := common.MapStr{
		"foo": "bar",
	}

	validator := Schema(Map{
		"non": Optional(IsEqual("foo")),
	})

	assertValidator(t, validator, m)
}

func TestExistence(t *testing.T) {
	m := common.MapStr{
		"exists": "foo",
	}

	validator := Schema(Map{
		"exists": KeyPresent,
		"non":    KeyMissing,
	})

	assertValidator(t, validator, m)
}

func TestComplex(t *testing.T) {
	m := common.MapStr{
		"foo": "bar",
		"hash": common.MapStr{
			"baz": 1,
			"bot": 2,
			"deep_hash": common.MapStr{
				"qux": "quark",
			},
		},
		"slice": []string{"pizza", "pasta", "and more"},
		"empty": nil,
		"arr":   []common.MapStr{{"foo": "bar"}, {"foo": "baz"}},
	}

	validator := Schema(Map{
		"foo": "bar",
		"hash": Map{
			"baz": 1,
			"bot": 2,
			"deep_hash": Map{
				"qux": "quark",
			},
		},
		"slice":        []string{"pizza", "pasta", "and more"},
		"empty":        KeyPresent,
		"doesNotExist": KeyMissing,
		"arr":          IsArrayOf(Schema(Map{"foo": IsStringContaining("a")})),
	})

	assertValidator(t, validator, m)
}

func TestLiteralArray(t *testing.T) {
	m := common.MapStr{
		"a": []interface{}{
			[]interface{}{1, 2, 3},
			[]interface{}{"foo", "bar"},
			"hello",
		},
	}

	validator := Schema(Map{
		"a": []interface{}{
			[]interface{}{1, 2, 3},
			[]interface{}{"foo", "bar"},
			"hello",
		},
	})

	goodRes, err := validator(m)
	require.NoError(t, err)

	assertResults(t, goodRes)
	// We evaluate multidimensional slice as a single field for now
	// This is kind of easier, but maybe we should do our own traversal later.
	assert.Len(t, goodRes.Fields, 6)
}

func TestStringSlice(t *testing.T) {
	m := common.MapStr{
		"a": []string{"a", "b"},
	}

	validator := Schema(Map{
		"a": []string{"a", "b"},
	})

	goodRes, err := validator(m)
	require.NoError(t, err)

	assertResults(t, goodRes)
	// We evaluate multidimensional slices as a single field for now
	// This is kind of easier, but maybe we should do our own traversal later.
	assert.Len(t, goodRes.Fields, 2)
}

func TestEmptySlice(t *testing.T) {
	// In the case of an empty Slice, the validator will compare slice type
	// In this case we're treating the slice as a value and doing a literal comparison
	// Users should use an IsDef testing for an empty slice (that can use reflection)
	// if they need something else.
	m := common.MapStr{
		"a": []interface{}{},
		"b": []string{},
	}

	validator := Schema(Map{
		"a": []interface{}{},
		"b": []string{},
	})

	goodRes, err := validator(m)
	require.NoError(t, err)

	assertResults(t, goodRes)
	assert.Len(t, goodRes.Fields, 2)
}

func TestLiteralMdSlice(t *testing.T) {
	m := common.MapStr{
		"a": [][]int{
			{1, 2, 3},
			{4, 5, 6},
		},
	}

	validator := Schema(Map{
		"a": [][]int{
			{1, 2, 3},
			{4, 5, 6},
		},
	})

	goodRes, err := validator(m)
	require.NoError(t, err)

	assertResults(t, goodRes)
	// We evaluate multidimensional slices as a single field for now
	// This is kind of easier, but maybe we should do our own traversal later.
	assert.Len(t, goodRes.Fields, 6)

	badValidator := Strict(Schema(Map{
		"a": [][]int{
			{1, 2, 3},
		},
	}))

	badRes, err := badValidator(m)
	require.NoError(t, err)

	assert.False(t, badRes.Valid)
	assert.Len(t, badRes.Fields, 7)
	// The reason the len is 4 is that there is 1 extra slice + 4 values.
	assert.Len(t, badRes.Errors(), 4)
}

func TestSliceOfIsDefs(t *testing.T) {
	m := common.MapStr{
		"a": []int{1, 2, 3},
		"b": []interface{}{"foo", "bar", 3},
	}

	goodV := Schema(Map{
		"a": []interface{}{IsIntGt(0), IsIntGt(1), 3},
		"b": []interface{}{IsStringContaining("o"), "bar", IsIntGt(2)},
	})

	assertValidator(t, goodV, m)

	badV := Schema(Map{
		"a": []interface{}{IsIntGt(100), IsIntGt(1), 3},
		"b": []interface{}{IsStringContaining("X"), "bar", IsIntGt(2)},
	})
	badRes, err := badV(m)
	require.NoError(t, err)

	assert.False(t, badRes.Valid)
	assert.Len(t, badRes.Errors(), 2)
}

func TestMatchArrayAsValue(t *testing.T) {
	m := common.MapStr{
		"a": []int{1, 2, 3},
		"b": []interface{}{"foo", "bar", 3},
	}

	goodV := Schema(Map{
		"a": []int{1, 2, 3},
		"b": []interface{}{"foo", "bar", 3},
	})

	assertValidator(t, goodV, m)

	badV := Schema(Map{
		"a": "robot",
		"b": []interface{}{"foo", "bar", 3},
	})

	badRes, err := badV(m)
	require.NoError(t, err)

	assert.False(t, badRes.Valid)
	assert.False(t, badRes.Fields["a"][0].Valid)
	for _, f := range badRes.Fields["b"] {
		assert.True(t, f.Valid)
	}
}

func TestErroringIsDef(t *testing.T) {
	m := common.MapStr{"foo": "bar"}

	expectedErr := fmt.Errorf("blahblah")

	v := Schema(Map{
		"foo": Is("This breaks", func(path Path, v interface{}) (*Results, error) {
			return nil, expectedErr
		}),
	})

	_, err := v(m)
	assert.Equal(t, expectedErr, err)
}

func TestInvalidPathIsdef(t *testing.T) {
	badPath := "foo...bar"
	m := common.MapStr{"foo": "bar"}
	v := Schema(Map{
		badPath: "invalid",
	})

	_, err := v(m)
	assert.Equal(t, InvalidPathString(badPath), err)
}
