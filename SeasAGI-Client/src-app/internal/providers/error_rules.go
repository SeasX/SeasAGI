package providers

import (
	"net/http"
	"strings"
)

type ErrorClass string

const (
	ErrorClassRetryable    ErrorClass = "retryable"
	ErrorClassRateLimit    ErrorClass = "rate_limit"
	ErrorClassAuth         ErrorClass = "auth"
	ErrorClassModel        ErrorClass = "model"
	ErrorClassContext      ErrorClass = "context_length"
	ErrorClassServer       ErrorClass = "server"
	ErrorClassPermanent    ErrorClass = "permanent"
	ErrorClassUnknown      ErrorClass = "unknown"
)

type ErrorRule struct {
	StatusCodes  []int
	Substrings   []string
	Class        ErrorClass
	Retryable    bool
}

var defaultErrorRules = []ErrorRule{
	{
		StatusCodes: []int{429},
		Class:       ErrorClassRateLimit,
		Retryable:   true,
	},
	{
		StatusCodes: []int{401, 403},
		Class:       ErrorClassAuth,
		Retryable:   false,
	},
	{
		StatusCodes: []int{404},
		Substrings:  []string{"model", "not found"},
		Class:       ErrorClassModel,
		Retryable:   false,
	},
	{
		StatusCodes: []int{400},
		Substrings:  []string{"context", "token", "length", "too many"},
		Class:       ErrorClassContext,
		Retryable:   false,
	},
	{
		StatusCodes: []int{400},
		Class:       ErrorClassPermanent,
		Retryable:   false,
	},
	{
		StatusCodes: []int{500, 502, 503, 504},
		Class:       ErrorClassServer,
		Retryable:   true,
	},
}

type ErrorClassifier struct {
	rules []ErrorRule
}

func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{rules: defaultErrorRules}
}

func (ec *ErrorClassifier) Classify(statusCode int, message string) (ErrorClass, bool) {
	lowerMsg := strings.ToLower(message)

	for _, rule := range ec.rules {
		statusMatch := false
		for _, code := range rule.StatusCodes {
			if statusCode == code {
				statusMatch = true
				break
			}
		}
		if !statusMatch {
			continue
		}

		if len(rule.Substrings) > 0 {
			substringMatch := false
			for _, sub := range rule.Substrings {
				if strings.Contains(lowerMsg, sub) {
					substringMatch = true
					break
				}
			}
			if !substringMatch {
				continue
			}
		}

		return rule.Class, rule.Retryable
	}

	if statusCode >= 500 {
		return ErrorClassServer, true
	}

	return ErrorClassUnknown, false
}

func (ec *ErrorClassifier) AddRule(rule ErrorRule) {
	ec.rules = append(ec.rules, rule)
}

func ClassifyHTTPError(statusCode int, message string) (ErrorClass, bool) {
	return NewErrorClassifier().Classify(statusCode, message)
}

func IsRetryableHTTPError(statusCode int, message string) bool {
	_, retryable := ClassifyHTTPError(statusCode, message)
	return retryable
}

func ShouldRotateKey(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests
}
