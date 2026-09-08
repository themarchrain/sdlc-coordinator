package github

import "testing"

func TestAPIBaseURLDerivesRESTHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"web domain derives to api host", "https://github.com", "https://api.github.com"},
		{"trailing slash trimmed", "https://github.com/", "https://api.github.com"},
		{"insecure web domain still derives", "http://github.com", "https://api.github.com"},
		{"www host derives too", "https://www.github.com", "https://api.github.com"},
		{"api host passes through", "https://api.github.com", "https://api.github.com"},
		{"ghe host passes through untouched", "https://ghe.example.com", "https://ghe.example.com"},
		{"httptest host passes through", "http://127.0.0.1:54321", "http://127.0.0.1:54321"},
		{"empty input stays empty", "", ""},
		{"unparseable input passes through", "://bad", "://bad"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := APIBaseURL(tc.in); got != tc.want {
				t.Fatalf("APIBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
