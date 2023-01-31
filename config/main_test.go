package config

import (
	"reflect"
	"strconv"
	"testing"
)

func TestSettings(t *testing.T) {
	for i, test := range []struct {
		input    string
		expected map[string]string
	}{
		{
			input: "{ foo: bar }",
			expected: map[string]string{
				"foo": "bar",
			},
		},
		{
			input: `{foo: "bar bla" }`,
			expected: map[string]string{
				"foo": "bar bla",
			},
		},
		{
			input: "{foo: bar, bla: baz}",
			expected: map[string]string{
				"foo": "bar",
				"bla": "baz",
			},
		},
		{
			input: `{body: "foo bar bla"}`,
			expected: map[string]string{
				"body": "foo bar bla",
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			p := &parser{input: []byte(test.input)}
			got, err := p.parseSettings()
			if err != nil {
				t.Fatalf("failed to parse '%s'. %s", test.input, err)
			}

			if !reflect.DeepEqual(got, test.expected) {
				t.Fatalf("failedt to parse '%s'. got: %#v, want: %#v", test.input, got, test.expected)
			}
		})
	}
}

func TestConfig(t *testing.T) {
	for i, test := range []struct {
		input    string
		expected map[string][]HandlerConfig
	}{
		{
			input: "static",
			expected: map[string][]HandlerConfig{
				"/": {
					{
						Name:     "static",
						Settings: nil,
					},
				},
			},
		},
		{
			input: `static{body: "foo bar bla"}`,
			expected: map[string][]HandlerConfig{
				"/": {
					{
						Name: "static",
						Settings: map[string]string{
							"body": "foo bar bla",
						},
					},
				},
			},
		},
		{
			input: "/api:static",
			expected: map[string][]HandlerConfig{
				"/api": {
					{
						Name:     "static",
						Settings: nil,
					},
				},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got, err := Parse([]byte(test.input))
			if err != nil {
				t.Fatalf("failed to parse '%s'. %s", test.input, err)
			}

			if !reflect.DeepEqual(got, test.expected) {
				t.Fatalf("failedt to parse '%s'. got: %#v, want: %#v", test.input, got, test.expected)
			}
		})
	}
}
