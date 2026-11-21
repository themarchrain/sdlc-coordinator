package picli

import "testing"

func TestNormalizeFinalResultAcceptsBareAndFencedJSON(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  string
		valid bool
	}{
		{name: "bare object", raw: `{"a":1}`, want: `{"a":1}`, valid: true},
		{name: "fenced", raw: "```json\n{\"a\":1}\n```", want: `{"a":1}`, valid: true},
		{name: "fenced after prose", raw: "说明如下\n```json\n{\"a\":1}\n```", want: `{"a":1}`, valid: true},
		{name: "not json", raw: "not json"},
		{name: "two fences", raw: "```json\n{\"a\":1}\n```\n```json\n{\"b\":2}\n```"},
		{name: "non json fence", raw: "```text\n{\"a\":1}\n```"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := normalizeFinalResult([]byte(test.raw))
			if ok != test.valid {
				t.Fatalf("normalizeFinalResult(%q) ok = %v, want %v", test.raw, ok, test.valid)
			}
			if ok && string(got) != test.want {
				t.Fatalf("normalizeFinalResult(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
}
