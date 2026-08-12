package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultConfigFile = ".gallowrc.json"

const (
	RuleOff   = "off"
	RuleWarn  = "warn"
	RuleError = "error"
)

type DuplicateCodeConfig struct {
	MinLines      int `json:"minLines"`
	MinTokens     int `json:"minTokens"`
	MinTokenLines int `json:"minTokenLines"`
	MinStatements int `json:"minStatements"`
}

func (c DuplicateCodeConfig) Validate() error {
	for _, threshold := range []struct {
		name    string
		value   int
		minimum int
	}{
		{name: "duplicateCode.minLines", value: c.MinLines, minimum: 6},
		{name: "duplicateCode.minTokens", value: c.MinTokens, minimum: 50},
		{name: "duplicateCode.minTokenLines", value: c.MinTokenLines, minimum: 8},
		{name: "duplicateCode.minStatements", value: c.MinStatements, minimum: 1},
	} {
		if threshold.value != 0 && threshold.value < threshold.minimum {
			return fmt.Errorf("%s must be zero or at least %d", threshold.name, threshold.minimum)
		}
	}
	return nil
}

type Config struct {
	Root            string              `json:"root"`
	Format          string              `json:"format"`
	Production      *bool               `json:"production"`
	AllRequires     *bool               `json:"allRequires"`
	IgnoreGenerated *bool               `json:"ignoreGenerated"`
	Summary         *bool               `json:"summary"`
	FailOnIssues    *bool               `json:"failOnIssues"`
	Rules           map[string]string   `json:"rules"`
	IgnorePatterns  []string            `json:"ignorePatterns"`
	IgnoreFindings  []FindingMatcher    `json:"ignoreFindings"`
	Workspace       []string            `json:"workspace"`
	BuildTags       []string            `json:"buildTags"`
	DuplicateCode   DuplicateCodeConfig `json:"duplicateCode"`
}

type FindingMatcher struct {
	Type        string `json:"type"`
	File        string `json:"file"`
	Package     string `json:"package"`
	Module      string `json:"module"`
	ImportPath  string `json:"importPath"`
	Symbol      string `json:"symbol"`
	Receiver    string `json:"receiver"`
	Struct      string `json:"struct"`
	Fingerprint string `json:"fingerprint"`
}

func Load(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if _, ok := fields["health"]; ok {
		return Config{}, false, fmt.Errorf("parse config %s: setting %q was removed; remove maxCyclomatic/maxCognitive because Gallow no longer performs complexity analysis", path, "health")
	}

	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("validate config %s: %w", path, err)
	}

	return cfg, true, nil
}

func (c Config) Validate() error {
	switch c.Format {
	case "codeclimate", "gitlab-codequality", "annotations":
		return fmt.Errorf("format %q was removed; use %q for CI integrations or %q for a custom adapter", c.Format, "sarif", "json")
	}

	for rule, severity := range c.Rules {
		if removedFindingType(rule) {
			return fmt.Errorf("rule %q was removed from Gallow's repository-entropy scope", rule)
		}
		switch severity {
		case RuleOff, RuleWarn, RuleError:
		default:
			return fmt.Errorf("rule %q has unsupported severity %q", rule, severity)
		}
	}
	if err := c.DuplicateCode.Validate(); err != nil {
		return err
	}
	for _, matcher := range c.IgnoreFindings {
		if removedFindingType(matcher.Type) {
			return fmt.Errorf("ignore finding type %q was removed from Gallow's repository-entropy scope", matcher.Type)
		}
	}

	return nil
}

func removedFindingType(findingType string) bool {
	switch findingType {
	case "complexity", "test-only-dependency", "local-replace":
		return true
	default:
		return false
	}
}

func DefaultPath(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Join(root, DefaultConfigFile)
}
