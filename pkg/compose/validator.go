package compose

import (
	"io"

	"github.com/lissto-dev/controller/pkg/config"
	"github.com/sirupsen/logrus"
)

// warningHook captures warning messages from logrus
type warningHook struct {
	warnings []string
}

func (h *warningHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.WarnLevel}
}

func (h *warningHook) Fire(entry *logrus.Entry) error {
	h.warnings = append(h.warnings, entry.Message)
	return nil
}

// ValidationResult contains the result of compose file validation
type ValidationResult struct {
	Valid    bool
	Metadata *BlueprintMetadata
	Errors   []string
	Warnings []string
}

// ValidateCompose validates a compose file and returns detailed validation results
// This function is used by both the API and CLI for consistent validation
func ValidateCompose(composeContent string) (*ValidationResult, error) {
	return validateComposeInternal(composeContent, true)
}

// ValidateComposeRaw validates a compose file without capturing warnings
// Warnings will be output directly to the logger (useful for debugging)
func ValidateComposeRaw(composeContent string) (*ValidationResult, error) {
	return validateComposeInternal(composeContent, false)
}

// validateComposeInternal is the internal validation function
func validateComposeInternal(composeContent string, captureWarnings bool) (*ValidationResult, error) {
	var hook *warningHook
	var originalLevel logrus.Level
	var originalHooks logrus.LevelHooks
	var originalOutput io.Writer

	if captureWarnings {
		// Create a hook to capture warnings
		hook = &warningHook{warnings: []string{}}

		// Store original hooks and level
		originalLevel = logrus.GetLevel()
		originalHooks = logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
		originalOutput = logrus.StandardLogger().Out

		// Set up to capture warnings silently (discard output, only capture in hook)
		logrus.SetLevel(logrus.WarnLevel)
		logrus.SetOutput(io.Discard)
		logrus.AddHook(hook)

		// Ensure we restore original state
		defer func() {
			logrus.SetLevel(originalLevel)
			logrus.SetOutput(originalOutput)
			logrus.StandardLogger().ReplaceHooks(originalHooks)
		}()
	}

	// Parse with validation
	metadata, err := ParseBlueprintMetadata(composeContent, config.RepoConfig{})

	result := &ValidationResult{
		Valid:    err == nil,
		Metadata: metadata,
		Errors:   []string{},
		Warnings: []string{},
	}

	if captureWarnings && hook != nil {
		result.Warnings = hook.warnings
	}

	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	return result, nil
}
