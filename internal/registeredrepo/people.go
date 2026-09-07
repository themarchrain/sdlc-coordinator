package registeredrepo

import (
	"strings"
)

type PeopleSettings struct {
	ReviewerMentions string
}

func ResolvePeople(repo *RegisteredRepository, platform PeopleSettings) PeopleSettings {
	if repo == nil {
		return platform
	}
	return PeopleSettings{
		ReviewerMentions: firstNonEmpty(repo.ReviewerMentions, platform.ReviewerMentions),
	}
}

func firstNonEmpty(repoValue, platformValue string) string {
	if strings.TrimSpace(repoValue) != "" {
		return repoValue
	}
	return platformValue
}
