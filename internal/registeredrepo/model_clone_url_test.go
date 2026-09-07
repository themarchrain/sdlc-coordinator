package registeredrepo

import "testing"

func TestRegisteredRepositoryCloneURL(t *testing.T) {
	cases := []struct {
		name string
		repo RegisteredRepository
		want string
	}{
		{"base without trailing slash", RegisteredRepository{BaseURL: "https://gitea.example.com:3000", PathWithNamespace: "group/project"}, "https://gitea.example.com:3000/group/project.git"},
		{"base with trailing slash", RegisteredRepository{BaseURL: "https://gitea.example.com:3000/", PathWithNamespace: "group/project"}, "https://gitea.example.com:3000/group/project.git"},
		{"base with path prefix", RegisteredRepository{BaseURL: "https://host/gitea", PathWithNamespace: "group/project"}, "https://host/gitea/group/project.git"},
		{"base with path prefix and trailing slash", RegisteredRepository{BaseURL: "https://host/gitea/", PathWithNamespace: "group/project"}, "https://host/gitea/group/project.git"},
		{"empty base url", RegisteredRepository{BaseURL: "  ", PathWithNamespace: "group/project"}, ""},
		{"empty path", RegisteredRepository{BaseURL: "https://host", PathWithNamespace: ""}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.repo.CloneURL(); got != tc.want {
				t.Fatalf("CloneURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
