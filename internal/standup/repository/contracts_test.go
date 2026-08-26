package repository_test

import (
	"VoiceStandup.ai/internal/standup/onboarding"
	"VoiceStandup.ai/internal/standup/repository"
)

var _ onboarding.OnboardingRepo = (*repository.Repository)(nil)
